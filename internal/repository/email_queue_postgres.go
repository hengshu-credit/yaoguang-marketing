package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// EmailQueueRepository implements domain.EmailQueueRepository
type EmailQueueRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB // Used for testing with sqlmock
}

// NewEmailQueueRepository creates a new EmailQueueRepository using workspace repository
func NewEmailQueueRepository(workspaceRepo domain.WorkspaceRepository) domain.EmailQueueRepository {
	return &EmailQueueRepository{
		workspaceRepo: workspaceRepo,
	}
}

// NewEmailQueueRepositoryWithDB creates a new EmailQueueRepository with a direct DB connection (for testing)
func NewEmailQueueRepositoryWithDB(db *sql.DB) domain.EmailQueueRepository {
	return &EmailQueueRepository{
		db: db,
	}
}

// getDB returns the database connection for a workspace
func (r *EmailQueueRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

// psql is a Squirrel StatementBuilder configured for PostgreSQL
var emailQueuePsql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// Enqueue adds emails to the queue
func (r *EmailQueueRepository) Enqueue(ctx context.Context, workspaceID string, entries []*domain.EmailQueueEntry) error {
	if len(entries) == 0 {
		return nil
	}

	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Use a transaction for batch insert
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := r.EnqueueTx(ctx, tx, entries); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// EnqueueTx adds emails to the queue within an existing transaction
func (r *EmailQueueRepository) EnqueueTx(ctx context.Context, tx *sql.Tx, entries []*domain.EmailQueueEntry) error {
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()

	insertBuilder := emailQueuePsql.
		Insert("email_queue").
		Columns(
			"id", "status", "priority", "source_type", "source_id",
			"integration_id", "provider_kind", "contact_email", "message_id",
			"template_id", "payload", "attempts", "max_attempts",
			"created_at", "updated_at",
		)

	for _, entry := range entries {
		// Generate ID if not set
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}

		// Set defaults
		if entry.Status == "" {
			entry.Status = domain.EmailQueueStatusPending
		}
		if entry.Priority == 0 {
			entry.Priority = domain.EmailQueuePriorityMarketing
		}
		if entry.MaxAttempts == 0 {
			entry.MaxAttempts = 3
		}

		entry.CreatedAt = now
		entry.UpdatedAt = now

		payloadJSON, err := json.Marshal(entry.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		insertBuilder = insertBuilder.Values(
			entry.ID, entry.Status, entry.Priority, entry.SourceType, entry.SourceID,
			entry.IntegrationID, entry.ProviderKind, entry.ContactEmail, entry.MessageID,
			entry.TemplateID, payloadJSON, entry.Attempts, entry.MaxAttempts,
			entry.CreatedAt, entry.UpdatedAt,
		)
	}

	insertBuilder = insertBuilder.Suffix("ON CONFLICT (id) DO NOTHING")
	query, args, err := insertBuilder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to insert queue entries: %w", err)
	}

	return nil
}

// EnqueueIntentTx inserts one queue row keyed by its logical delivery intent.
// The unique delivery_intent_id index makes a repeated orchestration call a
// no-op even when it supplies a different random queue ID.
func (r *EmailQueueRepository) EnqueueIntentTx(ctx context.Context, tx *sql.Tx, entry *domain.EmailQueueEntry) (bool, error) {
	if entry == nil {
		return false, fmt.Errorf("email queue entry is required")
	}
	if entry.DeliveryIntentID == "" {
		return false, fmt.Errorf("delivery_intent_id is required")
	}
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Status == "" {
		entry.Status = domain.EmailQueueStatusPending
	}
	if entry.Priority == 0 {
		entry.Priority = domain.EmailQueuePriorityMarketing
	}
	if entry.MaxAttempts == 0 {
		entry.MaxAttempts = 3
	}
	now := time.Now().UTC()
	entry.CreatedAt, entry.UpdatedAt = now, now
	payload, err := json.Marshal(entry.Payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal payload: %w", err)
	}

	result, err := tx.ExecContext(ctx, `INSERT INTO email_queue (
		id, delivery_intent_id, status, priority, source_type, source_id,
		integration_id, provider_kind, contact_email, message_id, template_id,
		payload, attempts, max_attempts, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	ON CONFLICT (delivery_intent_id) WHERE delivery_intent_id IS NOT NULL DO NOTHING`,
		entry.ID, entry.DeliveryIntentID, entry.Status, entry.Priority, entry.SourceType, entry.SourceID,
		entry.IntegrationID, entry.ProviderKind, entry.ContactEmail, entry.MessageID, entry.TemplateID,
		payload, entry.Attempts, entry.MaxAttempts, entry.CreatedAt, entry.UpdatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to insert delivery queue entry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to inspect delivery queue insert: %w", err)
	}
	return rows == 1, nil
}

const claimPendingEmailQueueSQL = `
	WITH candidates AS (
		SELECT queue.id
		FROM email_queue queue
		LEFT JOIN delivery_intents intent ON intent.id = queue.delivery_intent_id
		WHERE (
			(queue.status = 'pending' AND (queue.next_retry_at IS NULL OR queue.next_retry_at <= CURRENT_TIMESTAMP))
			OR (queue.status = 'failed' AND queue.attempts < queue.max_attempts AND queue.next_retry_at <= CURRENT_TIMESTAMP)
			OR (queue.status = 'processing' AND queue.lease_expires_at <= CURRENT_TIMESTAMP)
		)
		AND (queue.delivery_intent_id IS NULL OR intent.status IN ('reserved', 'queued', 'transient_failed'))
		AND NOT EXISTS (
			SELECT 1 FROM delivery_attempts attempt
			WHERE attempt.intent_id = queue.delivery_intent_id
			  AND attempt.status IN ('submitting', 'provider_accepted', 'unknown')
		)
		ORDER BY queue.priority ASC, queue.created_at ASC
		LIMIT $1
		FOR UPDATE OF queue SKIP LOCKED
	)
	UPDATE email_queue queue
	SET status = 'processing', claim_token = $2, lease_expires_at = $3,
		attempts = queue.attempts + 1, updated_at = CURRENT_TIMESTAMP
	FROM candidates
	WHERE queue.id = candidates.id
	RETURNING queue.id, queue.status, queue.priority, queue.source_type, queue.source_id,
		queue.integration_id, queue.provider_kind, queue.contact_email, queue.message_id,
		queue.template_id, queue.payload, queue.attempts, queue.max_attempts, queue.last_error,
		queue.next_retry_at, queue.created_at, queue.updated_at, queue.processed_at,
		queue.delivery_intent_id, queue.claim_token, queue.lease_expires_at, queue.completed_at
`

// ClaimPending atomically chooses, marks and leases entries. SKIP LOCKED and
// UPDATE ... RETURNING ensure two workers can never receive the same claim.
func (r *EmailQueueRepository) ClaimPending(ctx context.Context, workspaceID string, limit int, leaseDuration time.Duration) ([]*domain.EmailQueueEntry, error) {
	if limit <= 0 {
		return []*domain.EmailQueueEntry{}, nil
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("lease duration must be positive")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}
	claimToken := uuid.New().String()
	leaseExpiresAt := time.Now().UTC().Add(leaseDuration)
	rows, err := db.QueryContext(ctx, claimPendingEmailQueueSQL, limit, claimToken, leaseExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending emails: %w", err)
	}
	defer rows.Close()

	entries := make([]*domain.EmailQueueEntry, 0)
	for rows.Next() {
		entry, scanErr := scanClaimedEmailQueueEntry(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating claimed emails: %w", err)
	}
	return entries, nil
}

func (r *EmailQueueRepository) CompleteClaim(ctx context.Context, workspaceID, id, claimToken string, completedAt time.Time) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE email_queue
		SET status = 'confirmed', completed_at = $3, processed_at = $3,
			claim_token = NULL, lease_expires_at = NULL, updated_at = $3
		WHERE id = $1 AND claim_token = $2`, id, claimToken, completedAt.UTC())
	if err != nil {
		return fmt.Errorf("failed to complete email queue claim: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect completed email queue claim: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("email queue claim was lost: %s", id)
	}
	return nil
}

func (r *EmailQueueRepository) FailClaim(ctx context.Context, workspaceID, id, claimToken, errorMsg string, nextRetryAt *time.Time) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE email_queue
		SET status = 'failed', last_error = $3, next_retry_at = $4,
			claim_token = NULL, lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND claim_token = $2`, id, claimToken, errorMsg, nextRetryAt)
	if err != nil {
		return fmt.Errorf("failed to fail email queue claim: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect failed email queue claim: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("email queue claim was lost: %s", id)
	}
	return nil
}

// FetchPending retrieves pending emails for processing
// Uses FOR UPDATE SKIP LOCKED for safe concurrent worker access
func (r *EmailQueueRepository) FetchPending(ctx context.Context, workspaceID string, limit int) ([]*domain.EmailQueueEntry, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Fetch pending emails ordered by priority (lower = higher priority), then by creation time
	// Include failed emails that are ready for retry
	// Include stuck processing entries (>2 minutes old) for recovery after worker crash
	query := `
		SELECT id, status, priority, source_type, source_id, integration_id, provider_kind,
		       contact_email, message_id, template_id, payload, attempts, max_attempts,
		       last_error, next_retry_at, created_at, updated_at, processed_at
		FROM email_queue
		WHERE (status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW()))
		   OR (status = 'failed' AND attempts < max_attempts AND next_retry_at <= NOW())
		   OR (status = 'processing' AND updated_at < NOW() - INTERVAL '2 minutes')
		ORDER BY priority ASC, created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending emails: %w", err)
	}
	defer rows.Close()

	var entries []*domain.EmailQueueEntry
	for rows.Next() {
		entry, err := scanEmailQueueEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entries, nil
}

// MarkAsProcessing atomically marks an entry as processing
func (r *EmailQueueRepository) MarkAsProcessing(ctx context.Context, workspaceID string, id string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		UPDATE email_queue
		SET status = 'processing', updated_at = NOW(), attempts = attempts + 1
		WHERE id = $1 AND (
			status IN ('pending', 'failed')
			OR (status = 'processing' AND updated_at < NOW() - INTERVAL '2 minutes')
		)
	`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark email as processing: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("email not found or already processing: %s", id)
	}

	return nil
}

// MarkAsSent deletes the entry after successful send
// (entries are removed immediately rather than kept with a "sent" status)
func (r *EmailQueueRepository) MarkAsSent(ctx context.Context, workspaceID string, id string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `DELETE FROM email_queue WHERE id = $1`

	_, err = db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete sent email: %w", err)
	}

	return nil
}

// MarkAsFailed marks an entry as failed and schedules retry
func (r *EmailQueueRepository) MarkAsFailed(ctx context.Context, workspaceID string, id string, errorMsg string, nextRetryAt *time.Time) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	now := time.Now().UTC()
	query := `
		UPDATE email_queue
		SET status = 'failed', updated_at = $2, last_error = $3, next_retry_at = $4
		WHERE id = $1
	`

	_, err = db.ExecContext(ctx, query, id, now, errorMsg, nextRetryAt)
	if err != nil {
		return fmt.Errorf("failed to mark email as failed: %w", err)
	}

	return nil
}

// Delete removes a queue entry (used when max retries exhausted)
func (r *EmailQueueRepository) Delete(ctx context.Context, workspaceID string, entryID string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	_, err = db.ExecContext(ctx, `DELETE FROM email_queue WHERE id = $1`, entryID)
	if err != nil {
		return fmt.Errorf("failed to delete queue entry: %w", err)
	}

	return nil
}

// SetNextRetry updates next_retry_at WITHOUT incrementing attempts
// Used by circuit breaker to schedule retry without burning retry attempts
func (r *EmailQueueRepository) SetNextRetry(ctx context.Context, workspaceID string, entryID string, nextRetry time.Time) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		UPDATE email_queue
		SET next_retry_at = $1, status = 'pending', updated_at = NOW()
		WHERE id = $2
	`

	_, err = db.ExecContext(ctx, query, nextRetry, entryID)
	if err != nil {
		return fmt.Errorf("failed to set next retry: %w", err)
	}

	return nil
}

// GetStats returns queue statistics for a workspace
func (r *EmailQueueRepository) GetStats(ctx context.Context, workspaceID string) (*domain.EmailQueueStats, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Note: sent entries are deleted immediately, so we don't track them in stats
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as pending,
			COALESCE(SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END), 0) as processing,
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) as failed
		FROM email_queue
	`

	var stats domain.EmailQueueStats
	err = db.QueryRowContext(ctx, query).Scan(
		&stats.Pending, &stats.Processing, &stats.Failed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	return &stats, nil
}

// GetBySourceID retrieves queue entries by source type and ID
func (r *EmailQueueRepository) GetBySourceID(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) ([]*domain.EmailQueueEntry, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		SELECT id, status, priority, source_type, source_id, integration_id, provider_kind,
		       contact_email, message_id, template_id, payload, attempts, max_attempts,
		       last_error, next_retry_at, created_at, updated_at, processed_at
		FROM email_queue
		WHERE source_type = $1 AND source_id = $2
		ORDER BY created_at ASC
	`

	rows, err := db.QueryContext(ctx, query, sourceType, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query by source: %w", err)
	}
	defer rows.Close()

	var entries []*domain.EmailQueueEntry
	for rows.Next() {
		entry, err := scanEmailQueueEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// CountBySourceAndStatus counts entries by source and status
func (r *EmailQueueRepository) CountBySourceAndStatus(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, status domain.EmailQueueStatus) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		SELECT COUNT(*)
		FROM email_queue
		WHERE source_type = $1 AND source_id = $2 AND status = $3
	`

	var count int64
	err = db.QueryRowContext(ctx, query, sourceType, sourceID, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count by source and status: %w", err)
	}

	return count, nil
}

// emailQueueExecutor is satisfied by both *sql.DB and *sql.Tx.
type emailQueueExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

const pauseBySourceSQL = `
	UPDATE email_queue
	SET status = 'paused', updated_at = NOW()
	WHERE source_type = $1 AND source_id = $2
	  AND status IN ('pending', 'failed')
`

const resumeBySourceSQL = `
	UPDATE email_queue
	SET status = 'pending', next_retry_at = NULL, updated_at = NOW()
	WHERE source_type = $1 AND source_id = $2
	  AND status = 'paused'
`

const deleteBySourceSQL = `
	DELETE FROM email_queue
	WHERE source_type = $1 AND source_id = $2
	  AND status IN ('pending', 'failed', 'paused')
`

func execBySource(ctx context.Context, exec emailQueueExecutor, query string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	result, err := exec.ExecContext(ctx, query, sourceType, sourceID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PauseBySource marks pending/failed entries for a source as paused.
func (r *EmailQueueRepository) PauseBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}
	n, err := execBySource(ctx, db, pauseBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to pause queue entries by source: %w", err)
	}
	return n, nil
}

// PauseBySourceTx is the transactional variant of PauseBySource.
func (r *EmailQueueRepository) PauseBySourceTx(ctx context.Context, tx *sql.Tx, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	n, err := execBySource(ctx, tx, pauseBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to pause queue entries by source: %w", err)
	}
	return n, nil
}

// ResumeBySource flips paused entries back to pending and clears next_retry_at.
func (r *EmailQueueRepository) ResumeBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}
	n, err := execBySource(ctx, db, resumeBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to resume queue entries by source: %w", err)
	}
	return n, nil
}

// ResumeBySourceTx is the transactional variant of ResumeBySource.
func (r *EmailQueueRepository) ResumeBySourceTx(ctx context.Context, tx *sql.Tx, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	n, err := execBySource(ctx, tx, resumeBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to resume queue entries by source: %w", err)
	}
	return n, nil
}

// DeleteBySource deletes pending/failed/paused entries for a source.
func (r *EmailQueueRepository) DeleteBySource(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}
	n, err := execBySource(ctx, db, deleteBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete queue entries by source: %w", err)
	}
	return n, nil
}

// DeleteBySourceTx is the transactional variant of DeleteBySource.
func (r *EmailQueueRepository) DeleteBySourceTx(ctx context.Context, tx *sql.Tx, sourceType domain.EmailQueueSourceType, sourceID string) (int64, error) {
	n, err := execBySource(ctx, tx, deleteBySourceSQL, sourceType, sourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete queue entries by source: %w", err)
	}
	return n, nil
}

// scanEmailQueueEntry scans a row into an EmailQueueEntry
func scanEmailQueueEntry(rows *sql.Rows) (*domain.EmailQueueEntry, error) {
	var entry domain.EmailQueueEntry
	var payloadJSON []byte
	var lastError sql.NullString
	var nextRetryAt sql.NullTime
	var processedAt sql.NullTime

	err := rows.Scan(
		&entry.ID, &entry.Status, &entry.Priority, &entry.SourceType, &entry.SourceID,
		&entry.IntegrationID, &entry.ProviderKind, &entry.ContactEmail, &entry.MessageID,
		&entry.TemplateID, &payloadJSON, &entry.Attempts, &entry.MaxAttempts,
		&lastError, &nextRetryAt, &entry.CreatedAt, &entry.UpdatedAt, &processedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan email queue entry: %w", err)
	}

	if lastError.Valid {
		entry.LastError = &lastError.String
	}
	if nextRetryAt.Valid {
		entry.NextRetryAt = &nextRetryAt.Time
	}
	if processedAt.Valid {
		entry.ProcessedAt = &processedAt.Time
	}

	if err := json.Unmarshal(payloadJSON, &entry.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &entry, nil
}

func scanClaimedEmailQueueEntry(rows *sql.Rows) (*domain.EmailQueueEntry, error) {
	var entry domain.EmailQueueEntry
	var payloadJSON []byte
	var lastError, deliveryIntentID sql.NullString
	var nextRetryAt, processedAt, leaseExpiresAt, completedAt sql.NullTime

	err := rows.Scan(
		&entry.ID, &entry.Status, &entry.Priority, &entry.SourceType, &entry.SourceID,
		&entry.IntegrationID, &entry.ProviderKind, &entry.ContactEmail, &entry.MessageID,
		&entry.TemplateID, &payloadJSON, &entry.Attempts, &entry.MaxAttempts,
		&lastError, &nextRetryAt, &entry.CreatedAt, &entry.UpdatedAt, &processedAt,
		&deliveryIntentID, &entry.ClaimToken, &leaseExpiresAt, &completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan claimed email queue entry: %w", err)
	}
	if lastError.Valid {
		entry.LastError = &lastError.String
	}
	if nextRetryAt.Valid {
		entry.NextRetryAt = &nextRetryAt.Time
	}
	if processedAt.Valid {
		entry.ProcessedAt = &processedAt.Time
	}
	if deliveryIntentID.Valid {
		entry.DeliveryIntentID = deliveryIntentID.String
	}
	if leaseExpiresAt.Valid {
		entry.LeaseExpiresAt = &leaseExpiresAt.Time
	}
	if completedAt.Valid {
		entry.CompletedAt = &completedAt.Time
	}
	if err := json.Unmarshal(payloadJSON, &entry.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal claimed email payload: %w", err)
	}
	return &entry, nil
}

const deleteForEmailSQL = `
	DELETE FROM email_queue
	WHERE contact_email = $1
`

// DeleteForEmail removes every queued email addressed to a contact.
//
// No status filter: see the interface doc for why 'processing' is included here
// but excluded from DeleteBySource.
func (r *EmailQueueRepository) DeleteForEmail(ctx context.Context, workspaceID string, email string) (int64, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	result, err := db.ExecContext(ctx, deleteForEmailSQL, email)
	if err != nil {
		return 0, fmt.Errorf("failed to delete queue entries for contact: %w", err)
	}
	return result.RowsAffected()
}
