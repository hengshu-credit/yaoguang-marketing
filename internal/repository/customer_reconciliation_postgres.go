package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const customerReconciliationBatchSize = 2_000

type customerReconciliationReference struct {
	entity     string
	table      string
	email      string
	keyColumns []string
}

var customerReconciliationReferences = []customerReconciliationReference{
	{entity: "contact_lists", table: "contact_lists", email: "email", keyColumns: []string{"email", "list_id"}},
	{entity: "contact_segments", table: "contact_segments", email: "email", keyColumns: []string{"email", "segment_id"}},
	{entity: "custom_events", table: "custom_events", email: "email", keyColumns: []string{"event_name", "external_id"}},
	{entity: "contact_timeline", table: "contact_timeline", email: "email", keyColumns: []string{"id"}},
	{entity: "contact_automations", table: "contact_automations", email: "contact_email", keyColumns: []string{"id"}},
	{entity: "automation_trigger_log", table: "automation_trigger_log", email: "contact_email", keyColumns: []string{"id"}},
	{entity: "message_history", table: "message_history", email: "contact_email", keyColumns: []string{"id"}},
	{entity: "email_queue", table: "email_queue", email: "contact_email", keyColumns: []string{"id"}},
}

type CustomerReconciliationPostgresRepository struct {
	workspaceRepository domain.WorkspaceRepository
}

var _ domain.CustomerReconciliationRepository = (*CustomerReconciliationPostgresRepository)(nil)

func NewCustomerReconciliationRepository(workspaceRepository domain.WorkspaceRepository) *CustomerReconciliationPostgresRepository {
	return &CustomerReconciliationPostgresRepository{workspaceRepository: workspaceRepository}
}

func (repository *CustomerReconciliationPostgresRepository) Run(
	ctx context.Context,
	workspaceID string,
	jobType domain.CustomerReconciliationJobType,
	batchSize int,
) (*domain.CustomerReconciliationRun, error) {
	if batchSize <= 0 {
		batchSize = customerReconciliationBatchSize
	}
	db, err := repository.workspaceRepository.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open customer reconciliation connection: %w", err)
	}
	defer conn.Close()

	lockName := "customer-reconciliation:" + string(jobType)
	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtext($1))", lockName).Scan(&locked); err != nil {
		return nil, fmt.Errorf("acquire customer reconciliation lock: %w", err)
	}
	if !locked {
		run, getErr := getCustomerReconciliationRun(ctx, conn, "", jobType)
		if getErr != nil {
			return nil, getErr
		}
		return run, nil
	}
	defer func() {
		var released bool
		_ = conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", lockName).Scan(&released)
	}()

	run, err := getCustomerReconciliationRun(ctx, conn, "", jobType)
	var notFound *domain.ErrCustomerReconciliationNotFound
	if errors.As(err, &notFound) {
		now := time.Now().UTC()
		run = &domain.CustomerReconciliationRun{
			ID: uuid.NewString(), JobType: jobType, Status: domain.CustomerReconciliationRunning,
			BatchSize: batchSize, Checkpoint: map[string]string{}, StartedAt: now, UpdatedAt: now,
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO customer_reconciliation_runs (
			id, job_type, status, batch_size, checkpoint, summary, started_at, updated_at
		) VALUES ($1, $2, 'running', $3, '{}'::jsonb, '{}'::jsonb, $4, $4)`, run.ID, run.JobType, run.BatchSize, now); err != nil {
			return nil, fmt.Errorf("create customer reconciliation run: %w", err)
		}
	} else if err != nil {
		return nil, err
	}

	repaired := map[string]int64{}
	if jobType == domain.CustomerReconciliationRepair {
		repaired, err = repairCustomerReferences(ctx, conn, run)
		if err != nil {
			return nil, repository.failRun(ctx, conn, run.ID, err)
		}
	}

	findings, err := scanCustomerProjectionFindings(ctx, conn)
	if err != nil {
		return nil, repository.failRun(ctx, conn, run.ID, err)
	}
	summary := customerReconciliationSummary{Findings: findings}
	for index := range findings {
		findings[index].RepairedCount = repaired[findings[index].EntityName]
		summary.MissingCount += findings[index].MissingCount
		summary.ConflictCount += findings[index].ConflictCount
		summary.RepairedCount += findings[index].RepairedCount
		if err := persistCustomerReconciliationFinding(ctx, conn, run.ID, jobType, findings[index]); err != nil {
			return nil, repository.failRun(ctx, conn, run.ID, err)
		}
	}
	summary.Findings = findings
	encodedSummary, err := json.Marshal(summary)
	if err != nil {
		return nil, repository.failRun(ctx, conn, run.ID, fmt.Errorf("encode customer reconciliation summary: %w", err))
	}
	completedAt := time.Now().UTC()
	if _, err := conn.ExecContext(ctx, `UPDATE customer_reconciliation_runs SET status = 'completed', summary = $2,
		last_error = NULL, updated_at = $3, completed_at = $3 WHERE id = $1`, run.ID, encodedSummary, completedAt); err != nil {
		return nil, fmt.Errorf("complete customer reconciliation run: %w", err)
	}
	return getCustomerReconciliationRun(ctx, conn, run.ID, "")
}

func (repository *CustomerReconciliationPostgresRepository) failRun(ctx context.Context, conn *sql.Conn, runID string, cause error) error {
	_, updateErr := conn.ExecContext(ctx, `UPDATE customer_reconciliation_runs SET status = 'failed', last_error = $2,
		updated_at = CURRENT_TIMESTAMP, completed_at = CURRENT_TIMESTAMP WHERE id = $1`, runID, cause.Error())
	if updateErr != nil {
		return fmt.Errorf("customer reconciliation failed: %v; persist failure: %w", cause, updateErr)
	}
	return cause
}

func (repository *CustomerReconciliationPostgresRepository) Get(ctx context.Context, workspaceID, runID string) (*domain.CustomerReconciliationRun, error) {
	db, err := repository.workspaceRepository.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return getCustomerReconciliationRun(ctx, db, runID, "")
}

type customerReconciliationQueryer interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func getCustomerReconciliationRun(ctx context.Context, queryer customerReconciliationQueryer, runID string, runningJobType domain.CustomerReconciliationJobType) (*domain.CustomerReconciliationRun, error) {
	query := "SELECT id, job_type, status, batch_size, checkpoint, summary, last_error, started_at, updated_at, completed_at FROM customer_reconciliation_runs"
	args := make([]interface{}, 0, 1)
	switch {
	case runID != "":
		query += " WHERE id = $1"
		args = append(args, runID)
	case runningJobType != "":
		query += " WHERE job_type = $1 AND status = 'running' ORDER BY started_at DESC, id DESC LIMIT 1"
		args = append(args, runningJobType)
	default:
		query += " ORDER BY started_at DESC, id DESC LIMIT 1"
	}
	row := queryer.QueryRowContext(ctx, query, args...)
	run := &domain.CustomerReconciliationRun{}
	var checkpointJSON, summaryJSON []byte
	var lastError sql.NullString
	var completedAt sql.NullTime
	if err := row.Scan(&run.ID, &run.JobType, &run.Status, &run.BatchSize, &checkpointJSON, &summaryJSON,
		&lastError, &run.StartedAt, &run.UpdatedAt, &completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &domain.ErrCustomerReconciliationNotFound{RunID: runID}
		}
		return nil, fmt.Errorf("get customer reconciliation run: %w", err)
	}
	run.Checkpoint = map[string]string{}
	if len(checkpointJSON) > 0 {
		if err := json.Unmarshal(checkpointJSON, &run.Checkpoint); err != nil {
			return nil, fmt.Errorf("decode customer reconciliation checkpoint: %w", err)
		}
	}
	if len(summaryJSON) > 0 && string(summaryJSON) != "{}" {
		var summary customerReconciliationSummary
		if err := json.Unmarshal(summaryJSON, &summary); err != nil {
			return nil, fmt.Errorf("decode customer reconciliation summary: %w", err)
		}
		run.Findings = summary.Findings
		run.MissingCount = summary.MissingCount
		run.ConflictCount = summary.ConflictCount
		run.RepairedCount = summary.RepairedCount
	}
	if lastError.Valid {
		run.LastError = lastError.String
	}
	if completedAt.Valid {
		value := completedAt.Time
		run.CompletedAt = &value
	}
	return run, nil
}

type customerReconciliationSummary struct {
	Findings      []domain.CustomerReconciliationFinding `json:"findings"`
	MissingCount  int64                                  `json:"missing_count"`
	ConflictCount int64                                  `json:"conflict_count"`
	RepairedCount int64                                  `json:"repaired_count"`
}

func scanCustomerProjectionFindings(ctx context.Context, queryer customerReconciliationQueryer) ([]domain.CustomerReconciliationFinding, error) {
	findings := make([]domain.CustomerReconciliationFinding, 0, len(customerReconciliationReferences)+3)
	specialQueries := []struct {
		entity     string
		query      string
		asConflict bool
	}{
		{entity: "contacts_without_customer", query: `SELECT COUNT(*) FROM contacts WHERE customer_id IS NULL`},
		{entity: "customers_without_contact", query: `SELECT COUNT(*) FROM customers customer
			WHERE customer.merged_into_id IS NULL
			AND EXISTS (SELECT 1 FROM customer_identities identity WHERE identity.customer_id = customer.id
				AND identity.identity_type = 'email' AND identity.is_primary AND identity.enabled)
			AND NOT EXISTS (SELECT 1 FROM contacts contact WHERE contact.customer_id = customer.id)`},
		{entity: "identity_contact_mismatch", asConflict: true, query: `SELECT COUNT(*) FROM contacts contact
			WHERE contact.customer_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM customer_identities identity WHERE identity.customer_id = contact.customer_id
				AND identity.identity_type = 'email' AND identity.is_primary AND identity.enabled)`},
	}
	for _, item := range specialQueries {
		finding := domain.CustomerReconciliationFinding{EntityName: item.entity}
		var count int64
		if err := queryer.QueryRowContext(ctx, item.query).Scan(&count); err != nil {
			return nil, fmt.Errorf("scan %s reconciliation: %w", item.entity, err)
		}
		if item.asConflict {
			finding.ConflictCount = count
			finding.UnrepairableCount = count
		} else {
			finding.MissingCount = count
			finding.UnrepairableCount = count
		}
		findings = append(findings, finding)
	}
	for _, reference := range customerReconciliationReferences {
		query := fmt.Sprintf(`SELECT
			COUNT(*) FILTER (WHERE legacy.customer_id IS NULL),
			COUNT(*) FILTER (WHERE legacy.customer_id IS NOT NULL AND contact.customer_id IS NOT NULL AND legacy.customer_id <> contact.customer_id),
			COUNT(*) FILTER (WHERE legacy.customer_id IS NULL AND contact.customer_id IS NOT NULL)
			FROM %s legacy LEFT JOIN contacts contact
			ON LOWER(BTRIM(legacy.%s)) = LOWER(BTRIM(contact.email))`, reference.table, reference.email)
		finding := domain.CustomerReconciliationFinding{EntityName: reference.entity}
		if err := queryer.QueryRowContext(ctx, query).Scan(&finding.MissingCount, &finding.ConflictCount, &finding.RepairableCount); err != nil {
			return nil, fmt.Errorf("scan %s reconciliation: %w", reference.entity, err)
		}
		finding.UnrepairableCount = finding.MissingCount - finding.RepairableCount + finding.ConflictCount
		findings = append(findings, finding)
	}
	return findings, nil
}

func repairCustomerReferences(ctx context.Context, conn *sql.Conn, run *domain.CustomerReconciliationRun) (map[string]int64, error) {
	repaired := make(map[string]int64, len(customerReconciliationReferences))
	for _, reference := range customerReconciliationReferences {
		cursor := run.Checkpoint[reference.entity]
		for {
			var nextCursor string
			var count int64
			if err := conn.QueryRowContext(ctx, repairCustomerReferenceBatchSQL(reference), cursor, run.BatchSize).Scan(&nextCursor, &count); err != nil {
				return nil, fmt.Errorf("repair %s customer references: %w", reference.entity, err)
			}
			if count == 0 {
				break
			}
			cursor = nextCursor
			repaired[reference.entity] += count
			run.Checkpoint[reference.entity] = cursor
			if _, err := conn.ExecContext(ctx, `UPDATE customer_reconciliation_runs
				SET checkpoint = jsonb_set(checkpoint, ARRAY[$2], to_jsonb($3::text), true), updated_at = CURRENT_TIMESTAMP
				WHERE id = $1`, run.ID, reference.entity, cursor); err != nil {
				return nil, fmt.Errorf("checkpoint %s customer repair: %w", reference.entity, err)
			}
		}
	}
	return repaired, nil
}

func repairCustomerReferenceBatchSQL(reference customerReconciliationReference) string {
	keyExpressions := make([]string, len(reference.keyColumns))
	selectedKeys := make([]string, len(reference.keyColumns))
	matchConditions := make([]string, len(reference.keyColumns))
	for index, column := range reference.keyColumns {
		keyExpressions[index] = "legacy." + column
		selectedKeys[index] = "legacy." + column
		matchConditions[index] = fmt.Sprintf("legacy.%s IS NOT DISTINCT FROM batch.%s", column, column)
	}
	cursorExpression := fmt.Sprintf("jsonb_build_array(%s)::text", strings.Join(keyExpressions, ", "))
	return fmt.Sprintf(`WITH batch AS (
		SELECT %s, %s AS cursor_key, contact.customer_id
		FROM %s legacy JOIN contacts contact
			ON LOWER(BTRIM(legacy.%s)) = LOWER(BTRIM(contact.email))
		WHERE legacy.customer_id IS NULL AND %s > $1
		ORDER BY cursor_key
		LIMIT $2
	), updated AS (
		UPDATE %s legacy SET customer_id = batch.customer_id
		FROM batch WHERE %s AND legacy.customer_id IS NULL
		RETURNING batch.cursor_key
	)
	SELECT COALESCE(MAX(cursor_key), $1), COUNT(*) FROM updated`,
		strings.Join(selectedKeys, ", "), cursorExpression, reference.table, reference.email,
		cursorExpression, reference.table, strings.Join(matchConditions, " AND "))
}

type customerReconciliationExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func persistCustomerReconciliationFinding(
	ctx context.Context,
	executor customerReconciliationExecutor,
	runID string,
	jobType domain.CustomerReconciliationJobType,
	finding domain.CustomerReconciliationFinding,
) error {
	_, err := executor.ExecContext(ctx, `INSERT INTO customer_projection_reconciliation (
		entity_name, missing_customer_id_count, conflict_count, last_scanned_at, last_repaired_at,
		last_error, updated_at, run_id
	) VALUES ($1, $2, $3, CURRENT_TIMESTAMP,
		CASE WHEN $4 = 'repair' THEN CURRENT_TIMESTAMP ELSE NULL END, NULL, CURRENT_TIMESTAMP, $5)
	ON CONFLICT (entity_name) DO UPDATE SET
		missing_customer_id_count = EXCLUDED.missing_customer_id_count,
		conflict_count = EXCLUDED.conflict_count,
		last_scanned_at = EXCLUDED.last_scanned_at,
		last_repaired_at = CASE WHEN $4 = 'repair' THEN CURRENT_TIMESTAMP ELSE customer_projection_reconciliation.last_repaired_at END,
		last_error = NULL, updated_at = EXCLUDED.updated_at, run_id = EXCLUDED.run_id`,
		finding.EntityName, finding.MissingCount, finding.ConflictCount, jobType, runID)
	if err != nil {
		return fmt.Errorf("persist %s reconciliation finding: %w", finding.EntityName, err)
	}
	return nil
}
