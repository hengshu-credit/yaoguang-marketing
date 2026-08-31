package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
)

type ImportJobPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewImportJobRepository(workspaceRepo domain.WorkspaceRepository) *ImportJobPostgresRepository {
	return &ImportJobPostgresRepository{workspaceRepo: workspaceRepo}
}
func NewImportJobRepositoryWithDB(db *sql.DB) *ImportJobPostgresRepository {
	return &ImportJobPostgresRepository{db: db}
}
func (r *ImportJobPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

func (r *ImportJobPostgresRepository) CreateImportJob(ctx context.Context, workspaceID string, job domain.ImportJob) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO import_jobs (
		id, status, filename, object_key, file_checksum, list_ids, total_count, pending_count, processing_count,
		succeeded_count, failed_count, created_at, updated_at
	) VALUES (NULLIF($1, '')::uuid, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $12)`,
		job.ID, job.Status, job.Filename, job.ObjectKey, job.FileChecksum, pq.Array(job.ListIDs), job.Counters.Total, job.Counters.Pending,
		job.Counters.Processing, job.Counters.Succeeded, job.Counters.Failed, job.CreatedAt)
	if err != nil {
		return fmt.Errorf("create import job: %w", err)
	}
	return nil
}

func (r *ImportJobPostgresRepository) StageImportRows(ctx context.Context, workspaceID, jobID string, rows []domain.ImportJobRow) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	ordinals := make([]int64, len(rows))
	payloads := make([]string, len(rows))
	checksums := make([]string, len(rows))
	statuses := make([]string, len(rows))
	errorCodes := make([]string, len(rows))
	var maxOrdinal int64
	for index, row := range rows {
		payload := row.RawPayload
		if !json.Valid(payload) {
			return 0, fmt.Errorf("import row %d payload is not valid json", row.Ordinal)
		}
		ordinals[index], payloads[index], checksums[index], statuses[index], errorCodes[index] =
			row.Ordinal, string(payload), row.Checksum, string(row.Status), row.ErrorCode
		if row.Ordinal > maxOrdinal {
			maxOrdinal = row.Ordinal
		}
	}
	var inserted, pending, failed int64
	var insertedMaxOrdinal sql.NullInt64
	err = tx.QueryRowContext(ctx, `WITH input AS (
		SELECT item.ordinal, item.raw_payload::jsonb AS raw_payload, item.row_checksum,
			item.status, NULLIF(item.error_code, '') AS error_code
		FROM unnest($2::bigint[], $3::text[], $4::text[], $5::text[], $6::text[])
			AS item(ordinal, raw_payload, row_checksum, status, error_code)
	), inserted AS (
		INSERT INTO import_job_rows (job_id, ordinal, raw_payload, row_checksum, status, error_code, error_detail)
		SELECT NULLIF($1, '')::uuid, ordinal, raw_payload, row_checksum, status, error_code, NULL FROM input
		ON CONFLICT (job_id, ordinal) DO NOTHING RETURNING ordinal, status
	) SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'pending'),
		COUNT(*) FILTER (WHERE status = 'failed'), MAX(ordinal) FROM inserted`,
		jobID, pq.Array(ordinals), pq.Array(payloads), pq.Array(checksums), pq.Array(statuses), pq.Array(errorCodes)).
		Scan(&inserted, &pending, &failed, &insertedMaxOrdinal)
	if err != nil {
		return 0, fmt.Errorf("stage import rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET total_count = total_count + $2,
		pending_count = pending_count + $3, failed_count = failed_count + $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND status = 'uploading'`, jobID, inserted, pending, failed); err != nil {
		return 0, fmt.Errorf("update staged import counters: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO import_job_checkpoints (job_id, staged_ordinal)
		VALUES (NULLIF($1, '')::uuid, $2) ON CONFLICT (job_id) DO UPDATE
		SET staged_ordinal = GREATEST(import_job_checkpoints.staged_ordinal, EXCLUDED.staged_ordinal), updated_at = CURRENT_TIMESTAMP`, jobID, maxOrdinal); err != nil {
		return 0, fmt.Errorf("update import staging checkpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (r *ImportJobPostgresRepository) CommitImportJob(ctx context.Context, workspaceID, jobID, fileChecksum string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE import_jobs SET status = CASE WHEN pending_count = 0 THEN 'completed' ELSE 'staged' END,
		file_checksum = $2, committed_at = CURRENT_TIMESTAMP, completed_at = CASE WHEN pending_count = 0 THEN CURRENT_TIMESTAMP ELSE NULL END,
		updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid AND status = 'uploading'`, jobID, fileChecksum)
	if err != nil {
		return fmt.Errorf("commit import job: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("import job is not uploadable")
	}
	return nil
}

func (r *ImportJobPostgresRepository) RejectImportJob(ctx context.Context, workspaceID, jobID, reason string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE import_job_rows SET status = 'failed', error_code = 'job_rejected',
		error_detail = $2, updated_at = CURRENT_TIMESTAMP WHERE job_id = NULLIF($1, '')::uuid AND status = 'pending'`, jobID, reason)
	if err != nil {
		return err
	}
	converted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status = 'rejected', rejection_reason = $2,
		pending_count = pending_count - $3, failed_count = failed_count + $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND status = 'uploading'`, jobID, reason, converted); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ImportJobPostgresRepository) ClaimImportRows(ctx context.Context, workspaceID, jobID string, limit int, lease time.Duration) ([]domain.ImportJobRow, string, error) {
	if limit <= 0 || lease <= 0 {
		return nil, "", errors.New("import claim limit and lease must be positive")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, "", err
	}
	token := uuid.New().String()
	rows, err := db.QueryContext(ctx, `WITH candidates AS (
		SELECT job_id, ordinal, status AS previous_status FROM import_job_rows
		WHERE job_id = NULLIF($1, '')::uuid AND (status = 'pending' OR (status = 'processing' AND lease_expires_at <= CURRENT_TIMESTAMP))
		ORDER BY ordinal LIMIT $2 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE import_job_rows row SET status = 'processing', claim_token = NULLIF($3, '')::uuid,
			lease_expires_at = $4, attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP
		FROM candidates WHERE row.job_id = candidates.job_id AND row.ordinal = candidates.ordinal
		RETURNING row.job_id, row.ordinal, row.raw_payload, row.row_checksum, row.status
	), counters AS (
		UPDATE import_jobs SET status = 'processing',
			pending_count = pending_count - (SELECT COUNT(*) FROM candidates WHERE previous_status = 'pending'),
			processing_count = processing_count + (SELECT COUNT(*) FROM candidates WHERE previous_status = 'pending'),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid RETURNING id
	)
	SELECT claimed.job_id, claimed.ordinal, claimed.raw_payload, claimed.row_checksum, claimed.status
	FROM claimed CROSS JOIN (SELECT COUNT(*) FROM counters) ensured`, jobID, limit, token, time.Now().UTC().Add(lease))
	if err != nil {
		return nil, "", fmt.Errorf("claim import rows: %w", err)
	}
	defer rows.Close()
	claimed := []domain.ImportJobRow{}
	for rows.Next() {
		var row domain.ImportJobRow
		if err := rows.Scan(&row.JobID, &row.Ordinal, &row.RawPayload, &row.Checksum, &row.Status); err != nil {
			return nil, "", err
		}
		claimed = append(claimed, row)
	}
	return claimed, token, rows.Err()
}

func (r *ImportJobPostgresRepository) CompleteImportRow(ctx context.Context, workspaceID, jobID string, ordinal int64, claimToken string, status domain.ImportRowStatus, customerID, action, errorCode string) error {
	if status != domain.ImportRowSucceeded && status != domain.ImportRowFailed {
		return errors.New("import row completion must be succeeded or failed")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE import_job_rows SET status = $4, customer_id = NULLIF($5, '')::uuid,
		action = NULLIF($6, ''), error_code = NULLIF($7, ''), claim_token = NULL, lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE job_id = NULLIF($1, '')::uuid AND ordinal = $2 AND claim_token = NULLIF($3, '')::uuid AND status = 'processing'`,
		jobID, ordinal, claimToken, status, customerID, action, errorCode)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("import row claim was lost")
	}
	succeeded, failed := 0, 0
	if status == domain.ImportRowSucceeded {
		succeeded = 1
	} else {
		failed = 1
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET processing_count = processing_count - 1,
		succeeded_count = succeeded_count + $2, failed_count = failed_count + $3,
		status = CASE WHEN pending_count = 0 AND processing_count = 1 THEN 'completed' ELSE status END,
		completed_at = CASE WHEN pending_count = 0 AND processing_count = 1 THEN CURRENT_TIMESTAMP ELSE completed_at END,
		updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid`, jobID, succeeded, failed); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ImportJobPostgresRepository) GetImportJob(ctx context.Context, workspaceID, jobID string) (*domain.ImportJob, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	job := &domain.ImportJob{}
	var objectKey, checksum sql.NullString
	err = db.QueryRowContext(ctx, `SELECT id, status, filename, object_key, file_checksum, list_ids,
		total_count, pending_count, processing_count, succeeded_count, failed_count, created_at, updated_at
		FROM import_jobs WHERE id = NULLIF($1, '')::uuid`, jobID).Scan(&job.ID, &job.Status, &job.Filename, &objectKey, &checksum,
		pq.Array(&job.ListIDs),
		&job.Counters.Total, &job.Counters.Pending, &job.Counters.Processing, &job.Counters.Succeeded, &job.Counters.Failed, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	job.ObjectKey, job.FileChecksum = objectKey.String, checksum.String
	return job, job.Counters.Validate()
}

func (r *ImportJobPostgresRepository) ListImportJobs(ctx context.Context, workspaceID string, limit, offset int) ([]domain.ImportJob, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_jobs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, status, filename, object_key, file_checksum, list_ids,
		total_count, pending_count, processing_count, succeeded_count, failed_count, created_at, updated_at
		FROM import_jobs ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.ImportJob, 0)
	for rows.Next() {
		var item domain.ImportJob
		var objectKey, checksum sql.NullString
		if err := rows.Scan(&item.ID, &item.Status, &item.Filename, &objectKey, &checksum, pq.Array(&item.ListIDs),
			&item.Counters.Total, &item.Counters.Pending, &item.Counters.Processing,
			&item.Counters.Succeeded, &item.Counters.Failed, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		item.ObjectKey, item.FileChecksum = objectKey.String, checksum.String
		if err := item.Counters.Validate(); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// CancelImportJob converts every non-terminal row into an explicit failure in
// the same transaction as the job status change. This preserves the row
// conservation invariant even when a worker lease is active.
func (r *ImportJobPostgresRepository) CancelImportJob(ctx context.Context, workspaceID, jobID, reason string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE import_job_rows SET status = 'failed',
		error_code = 'cancelled_by_user', error_detail = $2, claim_token = NULL,
		lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE job_id = NULLIF($1, '')::uuid AND status IN ('pending', 'processing')`, jobID, reason)
	if err != nil {
		return err
	}
	converted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	jobResult, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status = 'cancelled',
		failed_count = failed_count + $2, pending_count = 0, processing_count = 0,
		completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND status IN ('uploading', 'staged', 'processing')`, jobID, converted)
	if err != nil {
		return err
	}
	count, err := jobResult.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("import job is already terminal or does not exist")
	}
	return tx.Commit()
}

func (r *ImportJobPostgresRepository) ListImportJobErrors(ctx context.Context, workspaceID, jobID string, limit, offset int) ([]domain.ImportJobRow, int, error) {
	if limit <= 0 || limit > 10_000 {
		limit = 1_000
	}
	if offset < 0 {
		offset = 0
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_job_rows WHERE job_id = NULLIF($1, '')::uuid AND status = 'failed'`, jobID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT job_id, ordinal, raw_payload, row_checksum, status,
		COALESCE(customer_id::text, ''), COALESCE(action, ''), COALESCE(error_code, ''), COALESCE(error_detail, '')
		FROM import_job_rows WHERE job_id = NULLIF($1, '')::uuid AND status = 'failed'
		ORDER BY ordinal LIMIT $2 OFFSET $3`, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.ImportJobRow, 0)
	for rows.Next() {
		var item domain.ImportJobRow
		if err := rows.Scan(&item.JobID, &item.Ordinal, &item.RawPayload, &item.Checksum, &item.Status,
			&item.CustomerID, &item.Action, &item.ErrorCode, &item.ErrorDetail); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
