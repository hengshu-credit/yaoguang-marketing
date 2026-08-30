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
)

type DeliveryPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
	queueRepo     domain.EmailQueueRepository
}

func NewDeliveryRepository(workspaceRepo domain.WorkspaceRepository, queueRepo domain.EmailQueueRepository) *DeliveryPostgresRepository {
	return &DeliveryPostgresRepository{workspaceRepo: workspaceRepo, queueRepo: queueRepo}
}

func NewDeliveryRepositoryWithDB(db *sql.DB) *DeliveryPostgresRepository {
	return &DeliveryPostgresRepository{db: db, queueRepo: &EmailQueueRepository{db: db}}
}

func (r *DeliveryPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

const deliveryIntentColumnsSQL = `id, effect_key, request_hash, source_type, source_id,
	source_version, customer_id, legacy_identity, channel, template_id, template_version,
	node_or_phase, occurrence, variant, status, suppression_reason, metadata, created_at, updated_at`

const insertDeliveryIntentSQL = `INSERT INTO delivery_intents (
	id, effect_key, request_hash, source_type, source_id, source_version,
	customer_id, legacy_identity, channel, template_id, template_version,
	node_or_phase, occurrence, variant, status, suppression_reason, metadata
) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid, NULLIF($8, ''),
	$9, NULLIF($10, ''), NULLIF($11, 0), $12, $13, $14, $15, NULLIF($16, ''), $17)
ON CONFLICT (effect_key) DO NOTHING
RETURNING ` + deliveryIntentColumnsSQL

const selectDeliveryIntentByEffectKeyForUpdateSQL = `SELECT ` + deliveryIntentColumnsSQL + `
	FROM delivery_intents WHERE effect_key = $1 FOR UPDATE`

const selectDeliveryIntentByEffectKeySQL = `SELECT ` + deliveryIntentColumnsSQL + `
	FROM delivery_intents WHERE effect_key = $1`

type deliveryRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanDeliveryIntent(row deliveryRowScanner) (domain.DeliveryIntent, error) {
	var intent domain.DeliveryIntent
	var customerID, legacyIdentity, templateID, suppressionReason sql.NullString
	var templateVersion sql.NullInt64
	var metadata []byte
	err := row.Scan(
		&intent.ID, &intent.EffectKey, &intent.RequestHash, &intent.SourceType, &intent.SourceID,
		&intent.SourceVersion, &customerID, &legacyIdentity, &intent.Channel, &templateID,
		&templateVersion, &intent.NodeOrPhase, &intent.Occurrence, &intent.Variant,
		&intent.Status, &suppressionReason, &metadata, &intent.CreatedAt, &intent.UpdatedAt,
	)
	if err != nil {
		return domain.DeliveryIntent{}, err
	}
	if customerID.Valid {
		intent.CustomerID = customerID.String
	}
	if legacyIdentity.Valid {
		intent.LegacyIdentity = legacyIdentity.String
	}
	if templateID.Valid {
		intent.TemplateID = templateID.String
	}
	if templateVersion.Valid {
		intent.TemplateVersion = templateVersion.Int64
	}
	if suppressionReason.Valid {
		intent.SuppressionReason = suppressionReason.String
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &intent.Metadata); err != nil {
			return domain.DeliveryIntent{}, fmt.Errorf("decode delivery intent metadata: %w", err)
		}
	}
	return intent, nil
}

func reserveDeliveryIntentTx(ctx context.Context, tx *sql.Tx, intent domain.DeliveryIntent) (domain.DeliveryIntent, bool, error) {
	if intent.EffectKey == "" || intent.RequestHash == "" {
		return domain.DeliveryIntent{}, false, errors.New("delivery effect key and request hash are required")
	}
	if intent.ID == "" {
		intent.ID = uuid.New().String()
	}
	if intent.Status == "" {
		intent.Status = domain.DeliveryStatusReserved
	}
	// A policy decision is part of intent creation, not a later provider
	// transition. Persist terminal/deferred decisions directly so a retry sees
	// the same effect key without ever enqueueing a provider request.
	if intent.Status != domain.DeliveryStatusReserved &&
		intent.Status != domain.DeliveryStatusSuppressed &&
		intent.Status != domain.DeliveryStatusDeferred {
		return domain.DeliveryIntent{}, false, errors.New("new delivery intent must start reserved, suppressed or deferred")
	}
	metadata, err := json.Marshal(intent.Metadata)
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("encode delivery intent metadata: %w", err)
	}
	stored, scanErr := scanDeliveryIntent(tx.QueryRowContext(ctx, insertDeliveryIntentSQL,
		intent.ID, intent.EffectKey, intent.RequestHash, intent.SourceType, intent.SourceID,
		intent.SourceVersion, intent.CustomerID, intent.LegacyIdentity, intent.Channel, intent.TemplateID,
		intent.TemplateVersion, intent.NodeOrPhase, intent.Occurrence, intent.Variant, intent.Status,
		intent.SuppressionReason, metadata,
	))
	if scanErr == nil {
		return stored, true, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return domain.DeliveryIntent{}, false, fmt.Errorf("reserve delivery intent: %w", scanErr)
	}

	stored, err = scanDeliveryIntent(tx.QueryRowContext(ctx, selectDeliveryIntentByEffectKeyForUpdateSQL, intent.EffectKey))
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("load conflicting delivery intent: %w", err)
	}
	if stored.RequestHash != intent.RequestHash {
		return domain.DeliveryIntent{}, false, domain.ErrDeliveryIntentHashConflict
	}
	return stored, false, nil
}

func (r *DeliveryPostgresRepository) ReserveIntent(ctx context.Context, workspaceID string, intent domain.DeliveryIntent) (domain.DeliveryIntent, bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("begin delivery reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stored, created, err := reserveDeliveryIntentTx(ctx, tx, intent)
	if err != nil {
		return domain.DeliveryIntent{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("commit delivery reservation: %w", err)
	}
	return stored, created, nil
}

func (r *DeliveryPostgresRepository) ReserveAndEnqueue(ctx context.Context, workspaceID string, intent domain.DeliveryIntent, entry *domain.EmailQueueEntry) (domain.ReserveDeliveryResult, error) {
	var result domain.ReserveDeliveryResult
	if entry == nil {
		return result, errors.New("email queue entry is required")
	}
	if r.queueRepo == nil {
		return result, errors.New("email queue repository is required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return result, fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin reserve and enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stored, created, err := reserveDeliveryIntentTx(ctx, tx, intent)
	if err != nil {
		return result, err
	}
	result.Intent, result.Created = stored, created

	if stored.Status == domain.DeliveryStatusReserved || stored.Status == domain.DeliveryStatusTransientFailed {
		entry.DeliveryIntentID = stored.ID
		queueCreated, enqueueErr := r.queueRepo.EnqueueIntentTx(ctx, tx, entry)
		if enqueueErr != nil {
			return result, enqueueErr
		}
		result.QueueCreated = queueCreated
		transition, updateErr := tx.ExecContext(ctx, `UPDATE delivery_intents
			SET status = 'queued', updated_at = CURRENT_TIMESTAMP
			WHERE id = $1 AND status IN ('reserved', 'transient_failed')`, stored.ID)
		if updateErr != nil {
			return result, fmt.Errorf("mark delivery intent queued: %w", updateErr)
		}
		rows, rowsErr := transition.RowsAffected()
		if rowsErr != nil {
			return result, fmt.Errorf("inspect delivery intent queue transition: %w", rowsErr)
		}
		if rows != 1 {
			return result, errors.New("delivery intent queue transition was rejected")
		}
		result.Intent.Status = domain.DeliveryStatusQueued
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit reserve and enqueue: %w", err)
	}
	return result, nil
}

func (r *DeliveryPostgresRepository) GetIntentByEffectKey(ctx context.Context, workspaceID, effectKey string) (*domain.DeliveryIntent, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace database: %w", err)
	}
	intent, err := scanDeliveryIntent(db.QueryRowContext(ctx, selectDeliveryIntentByEffectKeySQL, effectKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery intent by effect key: %w", err)
	}
	return &intent, nil
}

func (r *DeliveryPostgresRepository) ResolveCustomerID(ctx context.Context, workspaceID, email string) (string, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("get workspace database: %w", err)
	}
	var customerID sql.NullString
	err = db.QueryRowContext(ctx, `SELECT customer_id::text FROM contacts
		WHERE LOWER(BTRIM(email)) = LOWER(BTRIM($1::text))`, email).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve delivery customer by email: %w", err)
	}
	if customerID.Valid {
		return customerID.String, nil
	}
	return "", nil
}

func (r *DeliveryPostgresRepository) TransitionIntent(ctx context.Context, workspaceID, intentID string, from, to domain.DeliveryStatus, at time.Time) (bool, error) {
	if !from.CanTransitionTo(to) {
		return false, fmt.Errorf("delivery status transition %s to %s is not allowed", from, to)
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("get workspace database: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE delivery_intents SET status = $3, updated_at = $4
		WHERE id = $1 AND status = $2`, intentID, from, to, at.UTC())
	if err != nil {
		return false, fmt.Errorf("transition delivery intent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect delivery intent transition: %w", err)
	}
	return rows == 1, nil
}

func (r *DeliveryPostgresRepository) StartAttempt(ctx context.Context, workspaceID string, start domain.DeliveryAttemptStart) (domain.DeliveryAttempt, error) {
	var attempt domain.DeliveryAttempt
	if start.IntentID == "" || start.Provider == "" || start.ClaimToken == "" {
		return attempt, errors.New("intent, provider and claim token are required to start a delivery attempt")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return attempt, fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return attempt, fmt.Errorf("begin delivery attempt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var effectKey, requestHash string
	var intentStatus domain.DeliveryStatus
	if err := tx.QueryRowContext(ctx, `SELECT effect_key, request_hash, status
		FROM delivery_intents WHERE id = $1 FOR UPDATE`, start.IntentID).
		Scan(&effectKey, &requestHash, &intentStatus); err != nil {
		return attempt, fmt.Errorf("lock delivery intent for attempt: %w", err)
	}
	if intentStatus == domain.DeliveryStatusTransientFailed {
		if _, err := tx.ExecContext(ctx, `UPDATE delivery_intents SET status = 'queued', updated_at = CURRENT_TIMESTAMP
			WHERE id = $1 AND status = 'transient_failed'`, start.IntentID); err != nil {
			return attempt, fmt.Errorf("requeue transient delivery intent: %w", err)
		}
		intentStatus = domain.DeliveryStatusQueued
	}
	if intentStatus != domain.DeliveryStatusQueued {
		return attempt, fmt.Errorf("delivery intent %s cannot start from %s", start.IntentID, intentStatus)
	}

	var blocked bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM delivery_attempts WHERE intent_id = $1
		AND status IN ('submitting', 'provider_accepted', 'unknown')
	)`, start.IntentID).Scan(&blocked); err != nil {
		return attempt, fmt.Errorf("inspect active delivery attempt: %w", err)
	}
	if blocked {
		return attempt, fmt.Errorf("delivery intent %s already has an unresolved attempt", start.IntentID)
	}

	var attemptNo int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_no), 0) + 1
		FROM delivery_attempts WHERE intent_id = $1`, start.IntentID).Scan(&attemptNo); err != nil {
		return attempt, fmt.Errorf("allocate delivery attempt number: %w", err)
	}
	now := time.Now().UTC()
	attempt = domain.DeliveryAttempt{
		ID: uuid.New().String(), IntentID: start.IntentID, AttemptNo: attemptNo,
		Provider: start.Provider, RequestHash: requestHash, Status: domain.DeliveryStatusSubmitting,
		ClaimToken: start.ClaimToken, EffectKey: effectKey, SubmittedAt: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	if !start.LeaseExpiresAt.IsZero() {
		lease := start.LeaseExpiresAt.UTC()
		attempt.LeaseExpiresAt = &lease
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_attempts (
		id, intent_id, attempt_no, provider, request_hash, status, claim_token,
		lease_expires_at, submitted_at, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, 'submitting', NULLIF($6, '')::uuid, $7, $8, $8, $8)`,
		attempt.ID, attempt.IntentID, attempt.AttemptNo, attempt.Provider, attempt.RequestHash,
		attempt.ClaimToken, attempt.LeaseExpiresAt, now); err != nil {
		return domain.DeliveryAttempt{}, fmt.Errorf("insert delivery attempt: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_intents SET status = 'submitting', updated_at = $2
		WHERE id = $1 AND status = 'queued'`, start.IntentID, now)
	if err != nil {
		return domain.DeliveryAttempt{}, fmt.Errorf("mark delivery intent submitting: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		if rowsErr != nil {
			return domain.DeliveryAttempt{}, fmt.Errorf("inspect submitting transition: %w", rowsErr)
		}
		return domain.DeliveryAttempt{}, errors.New("delivery intent submitting transition was rejected")
	}
	if err := tx.Commit(); err != nil {
		return domain.DeliveryAttempt{}, fmt.Errorf("commit delivery attempt: %w", err)
	}
	return attempt, nil
}

func (r *DeliveryPostgresRepository) RecordAttemptOutcome(ctx context.Context, workspaceID, attemptID, claimToken string, outcome domain.DeliveryAttemptOutcome) error {
	if attemptID == "" || claimToken == "" {
		return errors.New("attempt and claim token are required")
	}
	switch outcome.Status {
	case domain.DeliveryStatusProviderAccepted, domain.DeliveryStatusConfirmed,
		domain.DeliveryStatusTransientFailed, domain.DeliveryStatusTerminalFailed, domain.DeliveryStatusUnknown:
	default:
		return fmt.Errorf("unsupported delivery attempt outcome: %s", outcome.Status)
	}
	when := outcome.OccurredAt.UTC()
	if outcome.OccurredAt.IsZero() {
		when = time.Now().UTC()
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delivery outcome: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var intentID string
	var attemptStatus domain.DeliveryStatus
	if err := tx.QueryRowContext(ctx, `SELECT intent_id, status FROM delivery_attempts
		WHERE id = $1 AND claim_token = NULLIF($2, '')::uuid FOR UPDATE`, attemptID, claimToken).
		Scan(&intentID, &attemptStatus); err != nil {
		return fmt.Errorf("lock delivery attempt outcome: %w", err)
	}
	if !attemptStatus.CanTransitionTo(outcome.Status) {
		return fmt.Errorf("delivery attempt transition %s to %s is not allowed", attemptStatus, outcome.Status)
	}
	var intentStatus domain.DeliveryStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM delivery_intents WHERE id = $1 FOR UPDATE`, intentID).
		Scan(&intentStatus); err != nil {
		return fmt.Errorf("lock delivery intent outcome: %w", err)
	}
	if !intentStatus.CanTransitionTo(outcome.Status) {
		return fmt.Errorf("delivery intent transition %s to %s is not allowed", intentStatus, outcome.Status)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE delivery_attempts SET
		status = $3::text, provider_message_id = NULLIF($4, ''),
		accepted_at = CASE WHEN $3::text = 'provider_accepted' THEN $5 ELSE accepted_at END,
		completed_at = CASE WHEN $3::text = 'confirmed' THEN $5 ELSE completed_at END,
		error_category = NULLIF($6, ''), error_code = NULLIF($7, ''), error_detail = NULLIF($8, ''),
		updated_at = $5
		WHERE id = $1 AND claim_token = NULLIF($2, '')::uuid`,
		attemptID, claimToken, outcome.Status, outcome.ProviderMessageID, when,
		outcome.ErrorCategory, outcome.ErrorCode, outcome.ErrorDetail); err != nil {
		return fmt.Errorf("update delivery attempt outcome: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE delivery_intents SET status = $2, updated_at = $3 WHERE id = $1`, intentID, outcome.Status, when); err != nil {
		return fmt.Errorf("update delivery intent outcome: %w", err)
	}

	var queueResult sql.Result
	switch outcome.Status {
	case domain.DeliveryStatusConfirmed:
		queueResult, err = tx.ExecContext(ctx, `UPDATE email_queue SET status = 'confirmed', completed_at = $3,
			processed_at = $3, claim_token = NULL, lease_expires_at = NULL, updated_at = $3
			WHERE delivery_intent_id = $1 AND claim_token = NULLIF($2, '')::uuid`, intentID, claimToken, when)
	case domain.DeliveryStatusTransientFailed:
		queueResult, err = tx.ExecContext(ctx, `UPDATE email_queue SET status = 'failed', last_error = $3,
			next_retry_at = $4, claim_token = NULL, lease_expires_at = NULL, updated_at = $5
			WHERE delivery_intent_id = $1 AND claim_token = NULLIF($2, '')::uuid`, intentID, claimToken, outcome.ErrorDetail, outcome.NextRetryAt, when)
	case domain.DeliveryStatusTerminalFailed, domain.DeliveryStatusUnknown:
		queueResult, err = tx.ExecContext(ctx, `UPDATE email_queue SET status = 'failed', last_error = $3,
			next_retry_at = NULL, claim_token = NULL, lease_expires_at = NULL, updated_at = $4
			WHERE delivery_intent_id = $1 AND claim_token = NULLIF($2, '')::uuid`, intentID, claimToken, outcome.ErrorDetail, when)
	}
	if err != nil {
		return fmt.Errorf("update delivery queue outcome: %w", err)
	}
	if queueResult != nil {
		rows, rowsErr := queueResult.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("inspect delivery queue outcome: %w", rowsErr)
		}
		if rows != 1 {
			return errors.New("delivery queue claim was lost before outcome")
		}
	}
	if outcome.Status == domain.DeliveryStatusUnknown {
		if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_reconciliations (
			intent_id, attempt_id, status, reason, next_query_at
		) VALUES ($1, $2, 'pending', $3, $4)`, intentID, attemptID, outcome.ErrorDetail, when); err != nil {
			return fmt.Errorf("enqueue unknown delivery reconciliation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delivery outcome: %w", err)
	}
	return nil
}
