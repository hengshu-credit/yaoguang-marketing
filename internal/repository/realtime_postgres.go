package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/domain"
)

const realtimeOutboxColumns = `
	o.id, o.event_id, o.topic, o.routing_key, o.payload, o.headers, o.status,
	o.attempts, o.available_at, o.claimed_by, o.claim_token, o.claim_expires_at,
	o.published_at, o.last_error, o.created_at`

type RealtimePostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewRealtimeRepository(workspaceRepo domain.WorkspaceRepository) domain.RealtimeRepository {
	return &RealtimePostgresRepository{workspaceRepo: workspaceRepo}
}

func NewRealtimeRepositoryWithDB(db *sql.DB) domain.RealtimeRepository {
	return &RealtimePostgresRepository{db: db}
}

func (r *RealtimePostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, fmt.Errorf("realtime repository has no workspace repository")
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection: %w", err)
	}
	return db, nil
}

func (r *RealtimePostgresRepository) AppendEvent(
	ctx context.Context,
	workspaceID string,
	envelope domain.EventEnvelope,
	receivedAt time.Time,
) (domain.EventAppendResult, error) {
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	} else {
		receivedAt = receivedAt.UTC()
	}
	if envelope.ID == uuid.Nil {
		envelope.ID = uuid.New()
	}
	if envelope.CorrelationID == uuid.Nil {
		envelope.CorrelationID = envelope.EventID
	}
	if envelope.SchemaVersion == 0 {
		envelope.SchemaVersion = 1
	}
	if len(envelope.Data) == 0 {
		envelope.Data = json.RawMessage(`{}`)
	}
	envelope.WorkspaceID = workspaceID
	envelope.ReceivedAt = receivedAt
	if err := envelope.Validate(); err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("validate event: %w", err)
	}

	payloadHash, err := eventBusinessPayloadHash(envelope)
	if err != nil {
		return domain.EventAppendResult{}, err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("marshal event envelope: %w", err)
	}
	contextJSON, err := json.Marshal(map[string]any{
		"correlation_id": envelope.CorrelationID,
		"causation_id":   envelope.CausationID,
		"trace_id":       envelope.TraceID,
	})
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("marshal event context: %w", err)
	}
	headers, err := json.Marshal(map[string]any{
		"schema_version": envelope.SchemaVersion,
		"correlation_id": envelope.CorrelationID,
		"causation_id":   envelope.CausationID,
		"trace_id":       envelope.TraceID,
	})
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("marshal event headers: %w", err)
	}

	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.EventAppendResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("begin event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var storedReceivedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO event_idempotency (id, received_at, payload_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
		RETURNING received_at
	`, envelope.EventID, receivedAt, payloadHash).Scan(&storedReceivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var existingHash string
		var messageID uuid.NullUUID
		lookupErr := tx.QueryRowContext(ctx, `
			SELECT i.payload_hash, i.received_at, o.id
			FROM event_idempotency i
			LEFT JOIN event_outbox o
			  ON o.event_id = i.id AND o.topic = 'notifuse.events'
			WHERE i.id = $1
			ORDER BY o.created_at
			LIMIT 1
		`, envelope.EventID).Scan(&existingHash, &storedReceivedAt, &messageID)
		if lookupErr != nil {
			return domain.EventAppendResult{}, fmt.Errorf("load existing event idempotency record: %w", lookupErr)
		}
		if existingHash != payloadHash {
			_ = tx.Rollback()
			return domain.EventAppendResult{}, domain.ErrEventPayloadConflict
		}
		if err := tx.Commit(); err != nil {
			return domain.EventAppendResult{}, fmt.Errorf("commit duplicate event lookup: %w", err)
		}
		return domain.EventAppendResult{
			EventID:    envelope.EventID,
			MessageID:  messageID.UUID,
			ReceivedAt: storedReceivedAt,
			Duplicate:  true,
		}, nil
	}
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("register event idempotency: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO event_ledger (
			id, event_type, subject_type, subject_id, contact_email, source,
			schema_version, occurred_at, received_at, properties, context
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11)
	`,
		envelope.EventID, envelope.Type, envelope.Subject.Type, envelope.Subject.ID,
		envelope.Subject.ContactEmail, envelope.Source, envelope.SchemaVersion,
		envelope.OccurredAt, receivedAt, []byte(envelope.Data), contextJSON,
	)
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("append event ledger: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO event_outbox (id, event_id, topic, routing_key, payload, headers)
		VALUES ($1, $2, 'notifuse.events', $3, $4, $5)
	`, envelope.ID, envelope.EventID, envelope.Type, payload, headers)
	if err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("append event outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.EventAppendResult{}, fmt.Errorf("commit event append: %w", err)
	}
	return domain.EventAppendResult{
		EventID:    envelope.EventID,
		MessageID:  envelope.ID,
		ReceivedAt: storedReceivedAt,
	}, nil
}

func eventBusinessPayloadHash(envelope domain.EventEnvelope) (string, error) {
	fingerprint, err := json.Marshal(struct {
		EventID       uuid.UUID           `json:"event_id"`
		Type          string              `json:"type"`
		SchemaVersion int                 `json:"schema_version"`
		Subject       domain.EventSubject `json:"subject"`
		Source        string              `json:"source"`
		OccurredAt    time.Time           `json:"occurred_at"`
		CorrelationID uuid.UUID           `json:"correlation_id"`
		CausationID   *uuid.UUID          `json:"causation_id,omitempty"`
		Data          json.RawMessage     `json:"data"`
	}{
		EventID:       envelope.EventID,
		Type:          envelope.Type,
		SchemaVersion: envelope.SchemaVersion,
		Subject:       envelope.Subject,
		Source:        envelope.Source,
		OccurredAt:    envelope.OccurredAt.UTC(),
		CorrelationID: envelope.CorrelationID,
		CausationID:   envelope.CausationID,
		Data:          envelope.Data,
	})
	if err != nil {
		return "", fmt.Errorf("marshal event payload fingerprint: %w", err)
	}
	hashValue, err := domain.CanonicalJSONHash(fingerprint)
	if err != nil {
		return "", fmt.Errorf("hash event payload: %w", err)
	}
	return hashValue, nil
}

func (r *RealtimePostgresRepository) ClaimOutbox(
	ctx context.Context,
	workspaceID, workerID string,
	now time.Time,
	lease time.Duration,
	limit int,
) ([]domain.OutboxMessage, error) {
	if limit <= 0 || lease <= 0 || workerID == "" {
		return nil, fmt.Errorf("worker id, positive lease, and positive limit are required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	claimToken := uuid.New()
	rows, err := db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM event_outbox
			WHERE available_at <= $1
			  AND (status = 'pending' OR (status = 'claimed' AND claim_expires_at <= $1))
			ORDER BY available_at, created_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE event_outbox AS o
		SET status = 'claimed',
			claimed_by = $3,
			claim_token = $4,
			claim_expires_at = $1 + $5::interval,
			attempts = o.attempts + 1
		FROM candidates
		WHERE o.id = candidates.id
		RETURNING `+realtimeOutboxColumns,
		now.UTC(), limit, workerID, claimToken, lease.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("claim realtime outbox: %w", err)
	}
	defer rows.Close()

	var messages []domain.OutboxMessage
	for rows.Next() {
		message, scanErr := scanOutboxMessage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan claimed outbox message: %w", scanErr)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox messages: %w", err)
	}
	return messages, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOutboxMessage(scanner rowScanner) (domain.OutboxMessage, error) {
	var message domain.OutboxMessage
	var payload, headers []byte
	var claimedBy, lastError sql.NullString
	var claimToken uuid.NullUUID
	var claimExpiresAt, publishedAt sql.NullTime
	if err := scanner.Scan(
		&message.ID, &message.EventID, &message.Topic, &message.RoutingKey,
		&payload, &headers, &message.Status, &message.Attempts, &message.AvailableAt,
		&claimedBy, &claimToken, &claimExpiresAt, &publishedAt, &lastError, &message.CreatedAt,
	); err != nil {
		return domain.OutboxMessage{}, err
	}
	message.Payload = json.RawMessage(payload)
	message.Headers = json.RawMessage(headers)
	if claimedBy.Valid {
		message.ClaimedBy = &claimedBy.String
	}
	if claimToken.Valid {
		value := claimToken.UUID
		message.ClaimToken = &value
	}
	if claimExpiresAt.Valid {
		value := claimExpiresAt.Time
		message.ClaimExpiresAt = &value
	}
	if publishedAt.Valid {
		value := publishedAt.Time
		message.PublishedAt = &value
	}
	if lastError.Valid {
		message.LastError = &lastError.String
	}
	return message, nil
}

func (r *RealtimePostgresRepository) MarkOutboxPublished(
	ctx context.Context,
	workspaceID string,
	id, claimToken uuid.UUID,
	publishedAt time.Time,
) (bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = 'published', published_at = $3,
			claimed_by = NULL, claim_token = NULL, claim_expires_at = NULL,
			last_error = NULL
		WHERE id = $1 AND claim_token = $2 AND status = 'claimed'
	`, id, claimToken, publishedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("mark outbox published: %w", err)
	}
	return affected(result)
}

func (r *RealtimePostgresRepository) ReleaseOutbox(
	ctx context.Context,
	workspaceID string,
	id, claimToken uuid.UUID,
	availableAt time.Time,
	lastError string,
	dead bool,
) (bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `
		UPDATE event_outbox
		SET status = CASE WHEN $5 THEN 'dead' ELSE 'pending' END,
			available_at = $3, last_error = NULLIF($4, ''),
			claimed_by = NULL, claim_token = NULL, claim_expires_at = NULL
		WHERE id = $1 AND claim_token = $2 AND status = 'claimed'
	`, id, claimToken, availableAt.UTC(), lastError, dead)
	if err != nil {
		return false, fmt.Errorf("release outbox claim: %w", err)
	}
	return affected(result)
}

func (r *RealtimePostgresRepository) ClaimInbox(
	ctx context.Context,
	tx *sql.Tx,
	_ string,
	consumer string,
	messageID uuid.UUID,
	now time.Time,
	lease time.Duration,
) (domain.InboxClaim, error) {
	if tx == nil {
		return domain.InboxClaim{}, fmt.Errorf("inbox claim requires a transaction")
	}
	if consumer == "" || messageID == uuid.Nil || lease <= 0 {
		return domain.InboxClaim{}, fmt.Errorf("consumer, message id, and positive lease are required")
	}
	claimToken := uuid.New()
	var claim domain.InboxClaim
	var processedAt sql.NullTime
	var lastError sql.NullString
	err := tx.QueryRowContext(ctx, `
		WITH claimed AS (
			INSERT INTO consumer_inbox (
				consumer, message_id, status, attempts, claim_token, claim_expires_at
			) VALUES ($1, $2, 'processing', 1, $3, $4)
			ON CONFLICT (consumer, message_id) DO UPDATE
			SET status = 'processing', attempts = consumer_inbox.attempts + 1,
				claim_token = EXCLUDED.claim_token,
				claim_expires_at = EXCLUDED.claim_expires_at,
				processed_at = NULL, last_error = NULL
			WHERE consumer_inbox.status = 'failed'
			   OR (consumer_inbox.status = 'processing' AND consumer_inbox.claim_expires_at <= $5)
			RETURNING consumer, message_id, status, attempts, claim_token,
				claim_expires_at, processed_at, last_error, created_at
		)
		SELECT consumer, message_id, status, attempts, claim_token,
			claim_expires_at, processed_at, last_error, created_at, TRUE
		FROM claimed
		UNION ALL
		SELECT consumer, message_id, status, attempts, claim_token,
			claim_expires_at, processed_at, last_error, created_at, FALSE
		FROM consumer_inbox
		WHERE consumer = $1 AND message_id = $2
		  AND NOT EXISTS (SELECT 1 FROM claimed)
		LIMIT 1
	`, consumer, messageID, claimToken, now.UTC().Add(lease), now.UTC()).Scan(
		&claim.Consumer, &claim.MessageID, &claim.Status, &claim.Attempts,
		&claim.ClaimToken, &claim.ClaimExpiresAt, &processedAt, &lastError,
		&claim.CreatedAt, &claim.Acquired,
	)
	if err != nil {
		return domain.InboxClaim{}, fmt.Errorf("claim consumer inbox: %w", err)
	}
	if processedAt.Valid {
		claim.ProcessedAt = &processedAt.Time
	}
	if lastError.Valid {
		claim.LastError = &lastError.String
	}
	return claim, nil
}

func (r *RealtimePostgresRepository) CompleteInbox(
	ctx context.Context,
	tx *sql.Tx,
	_ string,
	consumer string,
	messageID, claimToken uuid.UUID,
	completedAt time.Time,
) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("inbox completion requires a transaction")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE consumer_inbox
		SET status = 'completed', processed_at = $5, last_error = NULL
		WHERE consumer = $1 AND message_id = $2
		  AND claim_token = $3 AND status = $4
	`, consumer, messageID, claimToken, string(domain.InboxStatusProcessing), completedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("complete consumer inbox: %w", err)
	}
	return affected(result)
}

func (r *RealtimePostgresRepository) ListTriggerBindings(
	ctx context.Context,
	workspaceID, eventType, subjectType string,
) ([]domain.TriggerBinding, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT automation_id, automation_version, event_type, subject_type,
			dependency_keys, condition_hash, compiled_condition, created_at
		FROM automation_trigger_bindings
		WHERE event_type = $1 AND subject_type = $2
		ORDER BY automation_id, automation_version DESC
	`, eventType, subjectType)
	if err != nil {
		return nil, fmt.Errorf("list realtime trigger bindings: %w", err)
	}
	defer rows.Close()

	var bindings []domain.TriggerBinding
	for rows.Next() {
		var binding domain.TriggerBinding
		var compiled []byte
		if err := rows.Scan(
			&binding.AutomationID, &binding.AutomationVersion, &binding.EventType,
			&binding.SubjectType, pq.Array(&binding.DependencyKeys), &binding.ConditionHash,
			&compiled, &binding.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan realtime trigger binding: %w", err)
		}
		binding.CompiledCondition = json.RawMessage(compiled)
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realtime trigger bindings: %w", err)
	}
	return bindings, nil
}

func (r *RealtimePostgresRepository) WriteMatchAudit(
	ctx context.Context,
	workspaceID string,
	audit domain.MatchAudit,
) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	if len(audit.Reason) == 0 {
		audit.Reason = json.RawMessage(`{}`)
	}
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = time.Now().UTC()
	}
	var decisionHash string
	err = db.QueryRowContext(ctx, `
		INSERT INTO automation_match_audit (
			event_id, automation_id, engine, matched, decision_hash,
			contact_automation_id, reason, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (event_id, automation_id, engine) DO UPDATE
		SET decision_hash = automation_match_audit.decision_hash
		WHERE automation_match_audit.decision_hash = EXCLUDED.decision_hash
		  AND automation_match_audit.matched = EXCLUDED.matched
		RETURNING decision_hash
	`, audit.EventID, audit.AutomationID, audit.Engine, audit.Matched, audit.DecisionHash,
		audit.ContactAutomationID, []byte(audit.Reason), audit.CreatedAt.UTC()).Scan(&decisionHash)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrMatchAuditConflict
	}
	if err != nil {
		return fmt.Errorf("write automation match audit: %w", err)
	}
	return nil
}

func (r *RealtimePostgresRepository) ReserveSideEffect(
	ctx context.Context,
	workspaceID string,
	execution domain.SideEffectExecution,
) (domain.SideEffectExecution, bool, error) {
	if execution.EffectKey == "" || execution.ContactAutomationID == "" ||
		execution.AutomationVersion <= 0 || execution.NodeID == "" ||
		execution.Channel == "" || execution.RequestHash == "" {
		return domain.SideEffectExecution{}, false, fmt.Errorf("side effect identity and request hash are required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.SideEffectExecution{}, false, err
	}
	now := time.Now().UTC()
	if execution.Status == "" {
		execution.Status = domain.SideEffectStatusReserved
	}
	if execution.CreatedAt.IsZero() {
		execution.CreatedAt = now
	}
	if execution.UpdatedAt.IsZero() {
		execution.UpdatedAt = now
	}

	stored, err := scanSideEffect(db.QueryRowContext(ctx, `
		INSERT INTO side_effect_executions (
			effect_key, contact_automation_id, automation_version, node_id,
			execution_version, channel, status, provider_message_id,
			request_hash, attempts, last_error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (effect_key) DO NOTHING
		RETURNING effect_key, contact_automation_id, automation_version, node_id,
			execution_version, channel, status, provider_message_id,
			request_hash, attempts, last_error, created_at, updated_at
	`, execution.EffectKey, execution.ContactAutomationID, execution.AutomationVersion,
		execution.NodeID, execution.ExecutionVersion, execution.Channel, execution.Status,
		execution.ProviderMessageID, execution.RequestHash, execution.Attempts,
		execution.LastError, execution.CreatedAt, execution.UpdatedAt))
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.SideEffectExecution{}, false, fmt.Errorf("reserve side effect: %w", err)
	}

	stored, err = scanSideEffect(db.QueryRowContext(ctx, `
		SELECT effect_key, contact_automation_id, automation_version, node_id,
			execution_version, channel, status, provider_message_id,
			request_hash, attempts, last_error, created_at, updated_at
		FROM side_effect_executions WHERE effect_key = $1
	`, execution.EffectKey))
	if err != nil {
		return domain.SideEffectExecution{}, false, fmt.Errorf("load reserved side effect: %w", err)
	}
	if stored.RequestHash != execution.RequestHash {
		return domain.SideEffectExecution{}, false, domain.ErrSideEffectHashConflict
	}
	return stored, false, nil
}

func scanSideEffect(scanner rowScanner) (domain.SideEffectExecution, error) {
	var execution domain.SideEffectExecution
	var providerMessageID, lastError sql.NullString
	err := scanner.Scan(
		&execution.EffectKey, &execution.ContactAutomationID, &execution.AutomationVersion,
		&execution.NodeID, &execution.ExecutionVersion, &execution.Channel, &execution.Status,
		&providerMessageID, &execution.RequestHash, &execution.Attempts, &lastError,
		&execution.CreatedAt, &execution.UpdatedAt,
	)
	if err != nil {
		return domain.SideEffectExecution{}, err
	}
	if providerMessageID.Valid {
		execution.ProviderMessageID = &providerMessageID.String
	}
	if lastError.Valid {
		execution.LastError = &lastError.String
	}
	return execution, nil
}

func (r *RealtimePostgresRepository) GetEvent(
	ctx context.Context,
	workspaceID string,
	eventID uuid.UUID,
) (*domain.EventEnvelope, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var envelope domain.EventEnvelope
	var contactEmail sql.NullString
	var properties, eventContext []byte
	err = db.QueryRowContext(ctx, `
		SELECT id, event_type, subject_type, subject_id, contact_email, source,
			schema_version, occurred_at, received_at, properties, context
		FROM event_ledger
		WHERE id = $1
		ORDER BY received_at DESC
		LIMIT 1
	`, eventID).Scan(
		&envelope.EventID, &envelope.Type, &envelope.Subject.Type, &envelope.Subject.ID,
		&contactEmail, &envelope.Source, &envelope.SchemaVersion, &envelope.OccurredAt,
		&envelope.ReceivedAt, &properties, &eventContext,
	)
	if err != nil {
		return nil, fmt.Errorf("get realtime event: %w", err)
	}
	envelope.ID = envelope.EventID
	envelope.WorkspaceID = workspaceID
	envelope.Subject.ContactEmail = contactEmail.String
	envelope.Data = json.RawMessage(properties)
	envelope.CorrelationID = envelope.EventID
	applyEventContext(&envelope, eventContext)
	return &envelope, nil
}

func applyEventContext(envelope *domain.EventEnvelope, raw []byte) {
	var values struct {
		CorrelationID uuid.UUID  `json:"correlation_id"`
		CausationID   *uuid.UUID `json:"causation_id"`
		TraceID       string     `json:"trace_id"`
	}
	if json.Unmarshal(raw, &values) != nil {
		return
	}
	if values.CorrelationID != uuid.Nil {
		envelope.CorrelationID = values.CorrelationID
	}
	envelope.CausationID = values.CausationID
	envelope.TraceID = values.TraceID
}

func affected(result sql.Result) (bool, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

var _ domain.RealtimeRepository = (*RealtimePostgresRepository)(nil)
