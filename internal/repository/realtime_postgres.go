package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const realtimeOutboxColumns = `
	o.id, o.event_id, o.topic, o.routing_key, o.payload, o.headers, o.status,
	o.attempts, o.available_at, o.claimed_by, o.claim_token, o.claim_expires_at,
	o.published_at, o.last_error, o.created_at`

type RealtimePostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewRealtimeRepository(workspaceRepo domain.WorkspaceRepository) *RealtimePostgresRepository {
	return &RealtimePostgresRepository{workspaceRepo: workspaceRepo}
}

func NewRealtimeRepositoryWithDB(db *sql.DB) *RealtimePostgresRepository {
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
			id, event_type, subject_type, subject_id, customer_id, contact_email, source,
			schema_version, occurred_at, received_at, properties, context
		) VALUES ($1, $2, $3, $4,
			COALESCE(NULLIF($5, '')::uuid, (SELECT customer_id FROM contacts WHERE LOWER(BTRIM(email)) = LOWER(BTRIM($6)))),
			NULLIF($6, ''), $7, $8, $9, $10, $11, $12)
	`,
		envelope.EventID, envelope.Type, envelope.Subject.Type, envelope.Subject.ID,
		envelope.Subject.CustomerID, envelope.Subject.ContactEmail, envelope.Source, envelope.SchemaVersion,
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

func (r *RealtimePostgresRepository) ClaimConsumerMessage(
	ctx context.Context,
	workspaceID, consumer string,
	messageID uuid.UUID,
	now time.Time,
	lease time.Duration,
) (domain.InboxClaim, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.InboxClaim{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.InboxClaim{}, fmt.Errorf("begin consumer inbox claim: %w", err)
	}
	defer tx.Rollback()
	claim, err := r.ClaimInbox(ctx, tx, workspaceID, consumer, messageID, now, lease)
	if err != nil {
		return domain.InboxClaim{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.InboxClaim{}, fmt.Errorf("commit consumer inbox claim: %w", err)
	}
	return claim, nil
}

func (r *RealtimePostgresRepository) CompleteConsumerMessage(
	ctx context.Context,
	workspaceID, consumer string,
	messageID, claimToken uuid.UUID,
	completedAt time.Time,
) (bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin consumer inbox completion: %w", err)
	}
	defer tx.Rollback()
	completed, err := r.CompleteInbox(ctx, tx, workspaceID, consumer, messageID, claimToken, completedAt)
	if err != nil {
		return false, err
	}
	if !completed {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit consumer inbox completion: %w", err)
	}
	return true, nil
}

func (r *RealtimePostgresRepository) FailConsumerMessage(
	ctx context.Context,
	workspaceID, consumer string,
	messageID, claimToken uuid.UUID,
	failedAt time.Time,
	lastError string,
) (bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `
		UPDATE consumer_inbox
		SET status = 'failed', claim_expires_at = $5,
		    processed_at = NULL, last_error = NULLIF($6, '')
		WHERE consumer = $1 AND message_id = $2
		  AND claim_token = $3 AND status = $4
	`, consumer, messageID, claimToken, string(domain.InboxStatusProcessing), failedAt.UTC(), lastError)
	if err != nil {
		return false, fmt.Errorf("fail consumer inbox message: %w", err)
	}
	return affected(result)
}

func (r *RealtimePostgresRepository) ListTriggerBindings(
	ctx context.Context,
	workspaceID, eventType, subjectType string,
	dependencyKeys []string,
) ([]domain.TriggerBinding, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return listTriggerBindings(ctx, db, eventType, subjectType, dependencyKeys)
}

type realtimeQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listTriggerBindings(
	ctx context.Context,
	queryer realtimeQueryer,
	eventType, subjectType string,
	dependencyKeys []string,
) ([]domain.TriggerBinding, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT automation_id, automation_version, event_type, subject_type,
			b.dependency_keys, condition_hash, compiled_condition, b.created_at
		FROM automation_trigger_bindings b
		JOIN automations a ON a.id = b.automation_id
			AND a.version = b.automation_version
			AND a.status = 'live' AND a.deleted_at IS NULL
		WHERE event_type = $1 AND subject_type = $2
		  AND (cardinality(b.dependency_keys) = 0 OR b.dependency_keys && $3::text[])
		ORDER BY automation_id
	`, eventType, subjectType, pq.Array(dependencyKeys))
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

func (r *RealtimePostgresRepository) ReplaceTriggerBindings(
	ctx context.Context,
	workspaceID, automationID string,
	bindings []domain.TriggerBinding,
) error {
	if automationID == "" {
		return fmt.Errorf("automation id is required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin trigger binding replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceTriggerBindingsTx(ctx, tx, automationID, bindings); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit trigger binding replacement: %w", err)
	}
	return nil
}

func replaceTriggerBindingsTx(
	ctx context.Context,
	tx *sql.Tx,
	automationID string,
	bindings []domain.TriggerBinding,
) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM automation_trigger_bindings WHERE automation_id = $1`, automationID); err != nil {
		return fmt.Errorf("delete stale trigger bindings: %w", err)
	}
	for _, binding := range bindings {
		if binding.AutomationID != automationID || binding.AutomationVersion <= 0 ||
			binding.EventType == "" || binding.SubjectType == "" || binding.ConditionHash == "" ||
			len(binding.CompiledCondition) == 0 {
			return fmt.Errorf("invalid trigger binding for automation %s", automationID)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO automation_trigger_bindings (
				automation_id, automation_version, event_type, subject_type,
				dependency_keys, condition_hash, compiled_condition
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, binding.AutomationID, binding.AutomationVersion, binding.EventType,
			binding.SubjectType, pq.Array(binding.DependencyKeys), binding.ConditionHash,
			[]byte(binding.CompiledCondition)); err != nil {
			return fmt.Errorf("insert trigger binding: %w", err)
		}
	}
	return nil
}

func (r *RealtimePostgresRepository) DeleteTriggerBindings(
	ctx context.Context,
	workspaceID, automationID string,
) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM automation_trigger_bindings WHERE automation_id = $1`, automationID); err != nil {
		return fmt.Errorf("delete trigger bindings: %w", err)
	}
	return nil
}

func (r *RealtimePostgresRepository) ProcessRuleEvent(
	ctx context.Context,
	request domain.RuleProcessRequest,
) (domain.RuleProcessResult, error) {
	if request.WorkspaceID == "" || request.Consumer == "" || request.MessageID == uuid.Nil ||
		request.InboxLease <= 0 || !request.Engine.IsValid() {
		return domain.RuleProcessResult{}, fmt.Errorf("workspace, consumer, message id, engine, and inbox lease are required")
	}
	if request.Now.IsZero() {
		request.Now = time.Now().UTC()
	}
	db, err := r.getDB(ctx, request.WorkspaceID)
	if err != nil {
		return domain.RuleProcessResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RuleProcessResult{}, fmt.Errorf("begin rule event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	claim, err := r.ClaimInbox(
		ctx, tx, request.WorkspaceID, request.Consumer, request.MessageID,
		request.Now, request.InboxLease,
	)
	if err != nil {
		return domain.RuleProcessResult{}, err
	}
	if !claim.Acquired {
		if claim.Status == domain.InboxStatusCompleted {
			return domain.RuleProcessResult{Duplicate: true}, nil
		}
		return domain.RuleProcessResult{Busy: true}, nil
	}

	bindings, err := listTriggerBindings(
		ctx, tx, request.Envelope.Type, request.Envelope.Subject.Type, request.DependencyKeys,
	)
	if err != nil {
		return domain.RuleProcessResult{}, err
	}
	result := domain.RuleProcessResult{Candidates: len(bindings)}
	for _, binding := range bindings {
		matched, compiled, err := evaluateTriggerBinding(ctx, tx, binding, request.Envelope.Subject.ContactEmail)
		if err != nil {
			return domain.RuleProcessResult{}, fmt.Errorf("evaluate automation %s: %w", binding.AutomationID, err)
		}
		var customerID string
		if matched && (request.Primary || compiled.Frequency == domain.TriggerFrequencyOnce) {
			customerID, err = resolveRuleEventCustomerTx(ctx, tx, request.Envelope)
			if err != nil {
				return domain.RuleProcessResult{}, err
			}
		}
		if matched && !request.Primary && compiled.Frequency == domain.TriggerFrequencyOnce {
			if err := tx.QueryRowContext(ctx, `
				SELECT NOT EXISTS (
					SELECT 1 FROM journey_enrollments
					WHERE automation_id = $1 AND customer_id = $2::uuid AND frequency = 'once'
				)
			`, binding.AutomationID, customerID).Scan(&matched); err != nil {
				return domain.RuleProcessResult{}, fmt.Errorf("evaluate once frequency for automation %s: %w", binding.AutomationID, err)
			}
		}
		var contactAutomationID *string
		var entryOutcome domain.JourneyEntryOutcome
		if matched {
			result.Matched++
			if request.Primary {
				enrollment, err := enrollRealtimeAutomation(
					ctx, tx, request, binding, compiled, customerID,
				)
				if err != nil {
					return domain.RuleProcessResult{}, err
				}
				entryOutcome = enrollment.Outcome
				switch enrollment.Outcome {
				case domain.JourneyEntryOutcomeEnrolled:
					result.Enrolled++
					contactAutomationID = &enrollment.ContactAutomationID
				case domain.JourneyEntryOutcomeGuardDeferred:
					result.Deferred++
				case domain.JourneyEntryOutcomeAlreadyOnce,
					domain.JourneyEntryOutcomeReplayedEvent,
					domain.JourneyEntryOutcomeGuardDenied:
					result.Suppressed++
				}
			}
		}
		reason, _ := json.Marshal(map[string]any{
			"condition_hash": binding.ConditionHash,
			"decision":       map[bool]string{true: "matched", false: "not_matched"}[matched],
			"entry_outcome":  entryOutcome,
			"primary":        request.Primary,
		})
		audit := domain.MatchAudit{
			EventID:             request.Envelope.EventID,
			AutomationID:        binding.AutomationID,
			Engine:              request.Engine,
			Matched:             matched,
			DecisionHash:        ruleDecisionHash(binding.ConditionHash, matched),
			ContactAutomationID: contactAutomationID,
			Reason:              reason,
			CreatedAt:           request.Now,
		}
		if err := writeMatchAuditTx(ctx, tx, audit); err != nil {
			return domain.RuleProcessResult{}, err
		}
	}

	if result.Deferred > 0 {
		deferred, deferErr := tx.ExecContext(ctx, `
			UPDATE consumer_inbox
			SET status = 'failed', processed_at = NULL, last_error = 'journey_entry_deferred'
			WHERE consumer = $1 AND message_id = $2
				AND claim_token = $3 AND status = 'processing'
		`, request.Consumer, request.MessageID, claim.ClaimToken)
		if deferErr != nil {
			return domain.RuleProcessResult{}, fmt.Errorf("defer rule inbox for journey entry: %w", deferErr)
		}
		deferredRows, deferErr := deferred.RowsAffected()
		if deferErr != nil || deferredRows != 1 {
			return domain.RuleProcessResult{}, fmt.Errorf("rule inbox claim lost before journey entry defer")
		}
		if err := tx.Commit(); err != nil {
			return domain.RuleProcessResult{}, fmt.Errorf("commit deferred rule event: %w", err)
		}
		return result, nil
	}

	completed, err := r.CompleteInbox(
		ctx, tx, request.WorkspaceID, request.Consumer, request.MessageID,
		claim.ClaimToken, request.Now,
	)
	if err != nil {
		return domain.RuleProcessResult{}, err
	}
	if !completed {
		return domain.RuleProcessResult{}, fmt.Errorf("rule inbox claim lost before completion")
	}
	if err := tx.Commit(); err != nil {
		return domain.RuleProcessResult{}, fmt.Errorf("commit rule event: %w", err)
	}
	return result, nil
}

func evaluateTriggerBinding(
	ctx context.Context,
	tx *sql.Tx,
	binding domain.TriggerBinding,
	contactEmail string,
) (bool, domain.CompiledTriggerCondition, error) {
	var compiled domain.CompiledTriggerCondition
	if err := json.Unmarshal(binding.CompiledCondition, &compiled); err != nil {
		return false, compiled, fmt.Errorf("decode compiled condition: %w", err)
	}
	query := strings.TrimSpace(compiled.Query)
	if !strings.HasPrefix(strings.ToUpper(query), "SELECT ") || strings.Contains(query, ";") {
		return false, compiled, fmt.Errorf("compiled condition must be one SELECT statement")
	}
	arguments := append([]any(nil), compiled.Arguments...)
	if query != "SELECT TRUE" {
		arguments = append(arguments, contactEmail)
	}
	var matched bool
	if err := tx.QueryRowContext(ctx, query, arguments...).Scan(&matched); err != nil {
		return false, compiled, fmt.Errorf("execute compiled condition: %w", err)
	}
	return matched, compiled, nil
}

func enrollRealtimeAutomation(
	ctx context.Context,
	tx *sql.Tx,
	request domain.RuleProcessRequest,
	binding domain.TriggerBinding,
	compiled domain.CompiledTriggerCondition,
	customerID string,
) (domain.JourneyEnrollmentResult, error) {
	guard := domain.JourneyEntryGuard{}
	if compiled.Trigger != nil && compiled.Trigger.EntryGuard != nil {
		guard = *compiled.Trigger.EntryGuard
	}
	guardJSON, err := json.Marshal(guard)
	if err != nil {
		return domain.JourneyEnrollmentResult{}, fmt.Errorf("marshal journey entry guard: %w", err)
	}
	var enrollment domain.JourneyEnrollmentResult
	var contactAutomationID sql.NullString
	var retryAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT outcome, contact_automation_id, retry_at
		FROM automation_enroll_customer(
			$1, $2::uuid, $3, $4, $5, $6, $7::jsonb, $8, 'realtime'
		)
	`, binding.AutomationID, customerID, request.Envelope.Subject.ContactEmail,
		compiled.RootNodeID, string(compiled.Frequency), request.Envelope.EventID,
		string(guardJSON), binding.AutomationVersion).Scan(&enrollment.Outcome, &contactAutomationID, &retryAt)
	if err != nil {
		return domain.JourneyEnrollmentResult{}, fmt.Errorf("enroll realtime automation %s: %w", binding.AutomationID, err)
	}
	if !enrollment.Outcome.IsValid() {
		return domain.JourneyEnrollmentResult{}, fmt.Errorf("enroll realtime automation %s returned invalid outcome %q", binding.AutomationID, enrollment.Outcome)
	}
	if contactAutomationID.Valid {
		enrollment.ContactAutomationID = contactAutomationID.String
	}
	if retryAt.Valid {
		enrollment.RetryAt = &retryAt.Time
	}
	if enrollment.Outcome != domain.JourneyEntryOutcomeEnrolled {
		return enrollment, nil
	}
	if enrollment.ContactAutomationID == "" {
		return domain.JourneyEnrollmentResult{}, fmt.Errorf("enrolled realtime automation %s returned no contact automation id", binding.AutomationID)
	}

	messageID := uuid.New()
	causationID := request.Envelope.EventID
	payload, err := json.Marshal(domain.EventEnvelope{
		ID:            messageID,
		EventID:       request.Envelope.EventID,
		Type:          "journey.start",
		SchemaVersion: 1,
		Subject: domain.EventSubject{
			Type: "contact_automation", ID: enrollment.ContactAutomationID,
			CustomerID:   customerID,
			ContactEmail: request.Envelope.Subject.ContactEmail,
		},
		Source:        "rule-worker",
		OccurredAt:    request.Now.UTC(),
		ReceivedAt:    request.Now.UTC(),
		CorrelationID: request.Envelope.CorrelationID,
		CausationID:   &causationID,
		Data: mustJSON(map[string]any{
			"contact_automation_id": enrollment.ContactAutomationID,
			"automation_id":         binding.AutomationID,
			"automation_version":    binding.AutomationVersion,
		}),
	})
	if err != nil {
		return domain.JourneyEnrollmentResult{}, fmt.Errorf("marshal journey start command: %w", err)
	}
	headers, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"correlation_id": request.Envelope.CorrelationID,
		"causation_id":   request.Envelope.EventID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_outbox (id, event_id, topic, routing_key, payload, headers)
		VALUES ($1, $2, 'notifuse.jobs', $3, $4, $5)
	`, messageID, request.Envelope.EventID, "journey.start."+binding.AutomationID, payload, headers); err != nil {
		return domain.JourneyEnrollmentResult{}, fmt.Errorf("enqueue journey start command: %w", err)
	}
	return enrollment, nil
}

func resolveRuleEventCustomerTx(
	ctx context.Context,
	tx *sql.Tx,
	envelope domain.EventEnvelope,
) (string, error) {
	var customerID string
	var err error
	if strings.TrimSpace(envelope.Subject.CustomerID) != "" {
		if _, parseErr := uuid.Parse(envelope.Subject.CustomerID); parseErr != nil {
			return "", fmt.Errorf("%w: invalid customer_id", domain.ErrJourneyIdentityUnresolved)
		}
		err = tx.QueryRowContext(ctx, `
			SELECT COALESCE(merged_into_id, id)::text FROM customers WHERE id = $1::uuid
		`, envelope.Subject.CustomerID).Scan(&customerID)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT COALESCE(customer.merged_into_id, customer.id)::text
			FROM contacts contact JOIN customers customer ON customer.id = contact.customer_id
			WHERE LOWER(BTRIM(contact.email)) = LOWER(BTRIM($1))
		`, envelope.Subject.ContactEmail).Scan(&customerID)
	}
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(customerID) == "" {
		return "", fmt.Errorf("%w: no Customer for event subject", domain.ErrJourneyIdentityUnresolved)
	}
	if err != nil {
		return "", fmt.Errorf("resolve rule event Customer: %w", err)
	}
	return customerID, nil
}

func mustJSON(value any) json.RawMessage {
	payload, _ := json.Marshal(value)
	return payload
}

func writeMatchAuditTx(ctx context.Context, tx *sql.Tx, audit domain.MatchAudit) error {
	var decisionHash string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO automation_match_audit (
			event_id, automation_id, engine, matched, decision_hash,
			contact_automation_id, reason, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (event_id, automation_id, engine) DO UPDATE
		SET decision_hash = automation_match_audit.decision_hash,
			contact_automation_id = COALESCE(EXCLUDED.contact_automation_id, automation_match_audit.contact_automation_id),
			reason = EXCLUDED.reason,
			created_at = EXCLUDED.created_at
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

func ruleDecisionHash(conditionHash string, matched bool) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%t", conditionHash, matched)))
	return hex.EncodeToString(sum[:])
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
		SET decision_hash = automation_match_audit.decision_hash,
			contact_automation_id = COALESCE(EXCLUDED.contact_automation_id, automation_match_audit.contact_automation_id),
			reason = EXCLUDED.reason,
			created_at = EXCLUDED.created_at
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

func (r *RealtimePostgresRepository) SummarizeMatchAudits(
	ctx context.Context,
	workspaceID string,
	from time.Time,
	to time.Time,
) (domain.MatchReconciliationSummary, error) {
	if workspaceID == "" || from.IsZero() || to.IsZero() || !from.Before(to) {
		return domain.MatchReconciliationSummary{}, errors.New("workspace and valid audit window are required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.MatchReconciliationSummary{}, err
	}
	summary := domain.MatchReconciliationSummary{WorkspaceID: workspaceID, From: from.UTC(), To: to.UTC()}
	err = db.QueryRowContext(ctx, `
		WITH realtime AS (
			SELECT event_id, automation_id, matched
			FROM automation_match_audit
			WHERE engine = 'realtime' AND created_at >= $1 AND created_at < $2
		), legacy AS (
			SELECT event_id, automation_id, matched
			FROM automation_match_audit
			WHERE engine = 'legacy' AND created_at >= $1 AND created_at < $2
		), paired AS (
			SELECT r.event_id AS realtime_event_id, l.event_id AS legacy_event_id,
				r.matched AS realtime_matched, l.matched AS legacy_matched
			FROM realtime r
			FULL OUTER JOIN legacy l USING (event_id, automation_id)
		)
		SELECT
			COUNT(*) FILTER (WHERE realtime_event_id IS NOT NULL),
			COUNT(*) FILTER (WHERE legacy_event_id IS NOT NULL AND legacy_matched),
			COUNT(*) FILTER (WHERE realtime_event_id IS NOT NULL AND realtime_matched),
			COUNT(*) FILTER (
				WHERE realtime_event_id IS NOT NULL
				  AND realtime_matched = COALESCE(legacy_matched, FALSE)
			),
			COUNT(*) FILTER (
				WHERE realtime_event_id IS NOT NULL
				  AND realtime_matched <> COALESCE(legacy_matched, FALSE)
			),
			COUNT(*) FILTER (WHERE realtime_event_id IS NULL AND legacy_matched),
			COUNT(*) FILTER (WHERE realtime_event_id IS NOT NULL AND realtime_matched AND legacy_event_id IS NULL)
		FROM paired
	`, from.UTC(), to.UTC()).Scan(
		&summary.RealtimeEvaluated, &summary.LegacyMatched, &summary.RealtimeMatched,
		&summary.Agreements, &summary.DecisionMismatches, &summary.MissingRealtime,
		&summary.RealtimeOnlyMatched,
	)
	if err != nil {
		return domain.MatchReconciliationSummary{}, fmt.Errorf("summarize automation match audit: %w", err)
	}
	if summary.RealtimeEvaluated > 0 {
		summary.ConsistencyRate = float64(summary.Agreements) / float64(summary.RealtimeEvaluated)
	}
	return summary, nil
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

func (r *RealtimePostgresRepository) GetSideEffect(
	ctx context.Context,
	workspaceID string,
	effectKey string,
) (domain.SideEffectExecution, error) {
	if effectKey == "" {
		return domain.SideEffectExecution{}, errors.New("side effect key is required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.SideEffectExecution{}, err
	}
	execution, err := scanSideEffect(db.QueryRowContext(ctx, `
		SELECT effect_key, contact_automation_id, automation_version, node_id,
			execution_version, channel, status, provider_message_id,
			request_hash, attempts, last_error, created_at, updated_at
		FROM side_effect_executions WHERE effect_key = $1
	`, effectKey))
	if err != nil {
		return domain.SideEffectExecution{}, fmt.Errorf("get side effect: %w", err)
	}
	return execution, nil
}

func (r *RealtimePostgresRepository) TransitionSideEffect(
	ctx context.Context,
	workspaceID string,
	effectKey string,
	from domain.SideEffectStatus,
	to domain.SideEffectStatus,
	updatedAt time.Time,
	lastError *string,
) (bool, error) {
	if effectKey == "" || !validSideEffectTransition(from, to) {
		return false, fmt.Errorf("invalid side effect transition %q -> %q", from, to)
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := db.ExecContext(ctx, `
		UPDATE side_effect_executions
		SET status = $3,
			attempts = attempts + CASE WHEN $3 = 'submitted' THEN 1 ELSE 0 END,
			last_error = $4,
			updated_at = $5
		WHERE effect_key = $1 AND status = $2
	`, effectKey, from, to, lastError, updatedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("transition side effect: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read side effect transition result: %w", err)
	}
	return rows == 1, nil
}

func validSideEffectTransition(from, to domain.SideEffectStatus) bool {
	switch from {
	case domain.SideEffectStatusReserved, domain.SideEffectStatusFailed:
		return to == domain.SideEffectStatusSubmitted
	case domain.SideEffectStatusSubmitted:
		return to == domain.SideEffectStatusConfirmed || to == domain.SideEffectStatusFailed || to == domain.SideEffectStatusUnknown
	default:
		return false
	}
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
