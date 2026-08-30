package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *DeliveryPostgresRepository) ListDeliveries(ctx context.Context, request domain.DeliveryListRequest) ([]domain.DeliveryIntent, int, error) {
	if request.Limit <= 0 {
		request.Limit = 50
	}
	db, err := r.getDB(ctx, request.WorkspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("get workspace database: %w", err)
	}
	filterSQL := ` FROM delivery_intents intent WHERE
		($1 = '' OR intent.status = $1) AND ($2 = '' OR intent.channel = $2) AND
		($3 = '' OR intent.source_type = $3) AND ($4 = '' OR intent.source_id = $4) AND
		($5 = '' OR EXISTS (SELECT 1 FROM delivery_attempts attempt WHERE attempt.intent_id = intent.id AND LOWER(attempt.provider) = LOWER($5))) AND
		($6 = '' OR EXISTS (SELECT 1 FROM customers customer WHERE customer.id = intent.customer_id AND
			(customer.id::text = $6 OR customer.customer_no = $6 OR customer.external_user_id = $6))) AND
		($7::timestamptz IS NULL OR intent.created_at >= $7) AND ($8::timestamptz IS NULL OR intent.created_at <= $8)`
	args := []interface{}{request.Status, request.Channel, request.SourceType, request.SourceID, request.Provider, request.CustomerID, request.From, request.To}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*)`+filterSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count deliveries: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT `+deliveryIntentColumnsSQL+filterSQL+`
		ORDER BY intent.created_at DESC, intent.id DESC LIMIT $9 OFFSET $10`, append(args, request.Limit, request.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()
	items := make([]domain.DeliveryIntent, 0)
	for rows.Next() {
		item, scanErr := scanDeliveryIntent(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan delivery: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate deliveries: %w", err)
	}
	return items, total, nil
}

func (r *DeliveryPostgresRepository) GetDelivery(ctx context.Context, workspaceID, intentID string) (*domain.DeliveryDetail, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace database: %w", err)
	}
	intent, err := scanDeliveryIntent(db.QueryRowContext(ctx, `SELECT `+deliveryIntentColumnsSQL+` FROM delivery_intents WHERE id = $1`, intentID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrDeliveryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery intent: %w", err)
	}
	detail := &domain.DeliveryDetail{Intent: intent, Attempts: []domain.DeliveryAttempt{}, Reconciliations: []domain.DeliveryReconciliation{}}
	attemptRows, err := db.QueryContext(ctx, `SELECT id, intent_id, attempt_no, provider, request_hash,
		COALESCE(provider_message_id, ''), status, COALESCE(claim_token::text, ''), lease_expires_at,
		submitted_at, accepted_at, completed_at, COALESCE(error_category, ''), COALESCE(error_code, ''),
		COALESCE(error_detail, ''), created_at, updated_at
		FROM delivery_attempts WHERE intent_id = $1 ORDER BY attempt_no`, intentID)
	if err != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", err)
	}
	defer attemptRows.Close()
	for attemptRows.Next() {
		var attempt domain.DeliveryAttempt
		if err := attemptRows.Scan(&attempt.ID, &attempt.IntentID, &attempt.AttemptNo, &attempt.Provider, &attempt.RequestHash,
			&attempt.ProviderMessageID, &attempt.Status, &attempt.ClaimToken, &attempt.LeaseExpiresAt,
			&attempt.SubmittedAt, &attempt.AcceptedAt, &attempt.CompletedAt, &attempt.ErrorCategory,
			&attempt.ErrorCode, &attempt.ErrorDetail, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery attempt: %w", err)
		}
		detail.Attempts = append(detail.Attempts, attempt)
	}
	reconciliationRows, err := db.QueryContext(ctx, `SELECT id, intent_id, COALESCE(attempt_id::text, ''), status,
		COALESCE(resolution, ''), COALESCE(actor_id, ''), COALESCE(reason, ''), provider_result,
		next_query_at, COALESCE(lease_token::text, ''), lease_expires_at, created_at, updated_at, resolved_at
		FROM delivery_reconciliations WHERE intent_id = $1 ORDER BY created_at DESC`, intentID)
	if err != nil {
		return nil, fmt.Errorf("list delivery reconciliations: %w", err)
	}
	defer reconciliationRows.Close()
	for reconciliationRows.Next() {
		var item domain.DeliveryReconciliation
		var providerResult []byte
		if err := reconciliationRows.Scan(&item.ID, &item.IntentID, &item.AttemptID, &item.Status,
			&item.Resolution, &item.ActorID, &item.Reason, &providerResult, &item.NextQueryAt,
			&item.LeaseToken, &item.LeaseExpiresAt, &item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt); err != nil {
			return nil, fmt.Errorf("scan delivery reconciliation: %w", err)
		}
		if len(providerResult) > 0 {
			_ = json.Unmarshal(providerResult, &item.ProviderResult)
		}
		detail.Reconciliations = append(detail.Reconciliations, item)
	}
	return detail, nil
}

func (r *DeliveryPostgresRepository) RequestDeliveryReconciliation(ctx context.Context, workspaceID, intentID, reason string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace database: %w", err)
	}
	result, err := db.ExecContext(ctx, `INSERT INTO delivery_reconciliations (intent_id, attempt_id, status, reason, next_query_at)
		SELECT intent.id, attempt.id, 'pending', $2, CURRENT_TIMESTAMP
		FROM delivery_intents intent
		LEFT JOIN LATERAL (SELECT id FROM delivery_attempts WHERE intent_id = intent.id ORDER BY attempt_no DESC LIMIT 1) attempt ON TRUE
		WHERE intent.id = $1 AND intent.status IN ('submitting', 'provider_accepted', 'unknown')
		AND NOT EXISTS (SELECT 1 FROM delivery_reconciliations existing WHERE existing.intent_id = intent.id AND existing.status IN ('pending', 'querying'))`, intentID, reason)
	if err != nil {
		return fmt.Errorf("request delivery reconciliation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect delivery reconciliation request: %w", err)
	}
	if rows == 0 {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_intents WHERE id = $1)`, intentID).Scan(&exists); err != nil {
			return fmt.Errorf("inspect delivery intent: %w", err)
		}
		if !exists {
			return domain.ErrDeliveryNotFound
		}
	}
	return nil
}

func (r *DeliveryPostgresRepository) ResolveUnknownDelivery(ctx context.Context, workspaceID, intentID string, action domain.DeliveryResolutionAction, actorID, reason string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin unknown delivery resolution: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status domain.DeliveryStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM delivery_intents WHERE id = $1 FOR UPDATE`, intentID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return domain.ErrDeliveryNotFound
	} else if err != nil {
		return fmt.Errorf("lock unknown delivery: %w", err)
	}
	if status != domain.DeliveryStatusUnknown {
		return fmt.Errorf("delivery intent must be unknown, got %s", status)
	}
	var attemptID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM delivery_attempts WHERE intent_id = $1 AND status = 'unknown' ORDER BY attempt_no DESC LIMIT 1 FOR UPDATE`, intentID).Scan(&attemptID); err != nil {
		return fmt.Errorf("lock unknown delivery attempt: %w", err)
	}
	now := time.Now().UTC()
	var target domain.DeliveryStatus
	switch action {
	case domain.DeliveryResolutionMarkConfirmed:
		target = domain.DeliveryStatusConfirmed
	case domain.DeliveryResolutionMarkTerminalFailed:
		target = domain.DeliveryStatusTerminalFailed
	case domain.DeliveryResolutionRetryVerifiedNotAccepted:
		target = domain.DeliveryStatusQueued
	default:
		return errors.New("unsupported delivery resolution action")
	}
	attemptTarget := target
	if action == domain.DeliveryResolutionRetryVerifiedNotAccepted {
		attemptTarget = domain.DeliveryStatusTerminalFailed
	}
	if _, err := tx.ExecContext(ctx, `UPDATE delivery_attempts SET status = $2, error_category = 'manual_resolution',
		error_detail = $3, completed_at = CASE WHEN $2 IN ('confirmed', 'terminal_failed') THEN $4 ELSE completed_at END,
		updated_at = $4 WHERE id = $1`, attemptID, attemptTarget, reason, now); err != nil {
		return fmt.Errorf("resolve unknown delivery attempt: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE delivery_intents SET status = $2, updated_at = $3 WHERE id = $1`, intentID, target, now); err != nil {
		return fmt.Errorf("resolve unknown delivery intent: %w", err)
	}
	switch action {
	case domain.DeliveryResolutionMarkConfirmed:
		_, err = tx.ExecContext(ctx, `UPDATE email_queue SET status = 'confirmed', completed_at = $2, processed_at = $2,
			claim_token = NULL, lease_expires_at = NULL, updated_at = $2 WHERE delivery_intent_id = $1`, intentID, now)
	case domain.DeliveryResolutionRetryVerifiedNotAccepted:
		_, err = tx.ExecContext(ctx, `UPDATE email_queue SET status = 'failed', next_retry_at = $2,
			claim_token = NULL, lease_expires_at = NULL, updated_at = $2 WHERE delivery_intent_id = $1`, intentID, now)
	default:
		_, err = tx.ExecContext(ctx, `UPDATE email_queue SET status = 'failed', next_retry_at = NULL,
			claim_token = NULL, lease_expires_at = NULL, updated_at = $2 WHERE delivery_intent_id = $1`, intentID, now)
	}
	if err != nil {
		return fmt.Errorf("resolve unknown delivery queue: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_reconciliations (
		intent_id, attempt_id, status, resolution, actor_id, reason, provider_result, resolved_at, created_at, updated_at
	) VALUES ($1, $2, 'resolved', $3, $4, $5, '{}'::jsonb, $6, $6, $6)`, intentID, attemptID, action, actorID, reason, now); err != nil {
		return fmt.Errorf("audit unknown delivery resolution: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit unknown delivery resolution: %w", err)
	}
	return nil
}

func (r *DeliveryPostgresRepository) GetDeliveryProgress(ctx context.Context, workspaceID string, sourceType domain.DeliverySource, sourceID, sourceVersion string) (domain.DeliveryProgress, error) {
	var progress domain.DeliveryProgress
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return progress, fmt.Errorf("get workspace database: %w", err)
	}
	err = db.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COUNT(*) FILTER (WHERE status = 'planned'),
		COUNT(*) FILTER (WHERE status = 'reserved'),
		COUNT(*) FILTER (WHERE status = 'queued'),
		COUNT(*) FILTER (WHERE status = 'submitting'),
		COUNT(*) FILTER (WHERE status = 'provider_accepted'),
		COUNT(*) FILTER (WHERE status = 'confirmed'),
		COUNT(*) FILTER (WHERE status = 'suppressed'),
		COUNT(*) FILTER (WHERE status = 'deferred'),
		COUNT(*) FILTER (WHERE status IN ('transient_failed', 'terminal_failed')),
		COUNT(*) FILTER (WHERE status = 'unknown'),
		COUNT(*) FILTER (WHERE status = 'cancelled')
		FROM delivery_intents
		WHERE source_type = $1 AND source_id = $2 AND ($3 = '' OR source_version = $3)`,
		sourceType, sourceID, sourceVersion).Scan(
		&progress.AudienceTotal, &progress.Planned, &progress.Reserved, &progress.Queued,
		&progress.Submitting, &progress.Accepted, &progress.Confirmed, &progress.Suppressed,
		&progress.Deferred, &progress.Failed, &progress.Unknown, &progress.Cancelled,
	)
	if err != nil {
		return progress, fmt.Errorf("aggregate delivery progress: %w", err)
	}
	progress.Processed = progress.Confirmed + progress.Suppressed + progress.Failed + progress.Cancelled
	return progress, nil
}
