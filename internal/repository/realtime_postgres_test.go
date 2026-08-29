package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestRealtimeRepositoryClaimOutboxIsAtomicAndLeased(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Date(2026, 8, 29, 4, 0, 0, 0, time.UTC)
	id := uuid.New()
	eventID := uuid.New()
	rows := sqlmock.NewRows([]string{
		"id", "event_id", "topic", "routing_key", "payload", "headers", "status",
		"attempts", "available_at", "claimed_by", "claim_token", "claim_expires_at",
		"published_at", "last_error", "created_at",
	}).AddRow(
		id, eventID, "notifuse.events", "contact.updated", []byte(`{"event_id":"`+eventID.String()+`"}`),
		[]byte(`{"schema_version":1}`), "claimed", 1, now, "worker-1", uuid.New(), now.Add(30*time.Second),
		nil, nil, now,
	)
	mock.ExpectQuery(`(?s)WITH candidates AS .*FOR UPDATE SKIP LOCKED.*UPDATE event_outbox AS o.*claim_token.*RETURNING`).
		WithArgs(now, 2, "worker-1", sqlmock.AnyArg(), "30s").
		WillReturnRows(rows)

	repository := NewRealtimeRepositoryWithDB(db)
	claimed, err := repository.ClaimOutbox(context.Background(), "workspace-1", "worker-1", now, 30*time.Second, 2)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, id, claimed[0].ID)
	assert.Equal(t, domain.OutboxStatusClaimed, claimed[0].Status)
	assert.NotNil(t, claimed[0].ClaimToken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryMarkOutboxPublishedRequiresClaimToken(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	id := uuid.New()
	claimToken := uuid.New()
	publishedAt := time.Now().UTC()
	mock.ExpectExec(`(?s)UPDATE event_outbox.*status = 'published'.*id = \$1.*claim_token = \$2`).
		WithArgs(id, claimToken, publishedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	published, err := NewRealtimeRepositoryWithDB(db).MarkOutboxPublished(
		context.Background(), "workspace-1", id, claimToken, publishedAt,
	)
	require.NoError(t, err)
	assert.True(t, published)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryAppendEventWritesAuthorityAndOutboxInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Date(2026, 8, 29, 4, 0, 0, 0, time.UTC)
	eventID := uuid.New()
	envelope := domain.EventEnvelope{
		EventID:       eventID,
		Type:          "customer.plan.changed",
		SchemaVersion: 1,
		Subject:       domain.EventSubject{Type: "contact", ID: "ada@example.com", ContactEmail: "ada@example.com"},
		Source:        "crm",
		OccurredAt:    now.Add(-time.Minute),
		Data:          json.RawMessage(`{"plan":"pro"}`),
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO event_idempotency.*ON CONFLICT \(id\) DO NOTHING.*RETURNING received_at`).
		WithArgs(eventID, now, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"received_at"}).AddRow(now))
	mock.ExpectExec(`(?s)INSERT INTO event_ledger`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO event_outbox`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewRealtimeRepositoryWithDB(db).AppendEvent(
		context.Background(), "workspace-1", envelope, now,
	)
	require.NoError(t, err)
	assert.False(t, result.Duplicate)
	assert.Equal(t, eventID, result.EventID)
	assert.NotEqual(t, uuid.Nil, result.MessageID)
	assert.Equal(t, now, result.ReceivedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryAppendEventRejectsConflictingPayload(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	eventID := uuid.New()
	envelope := domain.EventEnvelope{
		EventID:       eventID,
		Type:          "contact.updated",
		SchemaVersion: 1,
		Subject:       domain.EventSubject{Type: "contact", ID: "ada@example.com"},
		Source:        "crm",
		OccurredAt:    now,
		Data:          json.RawMessage(`{"language":"fr"}`),
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO event_idempotency.*RETURNING received_at`).
		WillReturnRows(sqlmock.NewRows([]string{"received_at"}))
	mock.ExpectQuery(`(?s)SELECT i.payload_hash, i.received_at, o.id.*FROM event_idempotency i.*LEFT JOIN event_outbox o`).
		WithArgs(eventID).
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "received_at", "message_id"}).
			AddRow("different-payload-hash", now, uuid.New()))
	mock.ExpectRollback()

	_, err = NewRealtimeRepositoryWithDB(db).AppendEvent(context.Background(), "workspace-1", envelope, now)
	require.ErrorIs(t, err, domain.ErrEventPayloadConflict)
}

func TestRealtimeRepositoryAppendEventReturnsOriginalResultForDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Date(2026, 8, 29, 4, 0, 0, 0, time.UTC)
	eventID := uuid.New()
	messageID := uuid.New()
	envelope := domain.EventEnvelope{
		EventID:       eventID,
		Type:          "contact.updated",
		SchemaVersion: 1,
		Subject:       domain.EventSubject{Type: "contact", ID: "ada@example.com"},
		Source:        "crm",
		OccurredAt:    now,
		Data:          json.RawMessage(`{"language":"fr"}`),
	}
	fingerprintEnvelope := envelope
	fingerprintEnvelope.CorrelationID = eventID
	payloadHash, err := eventBusinessPayloadHash(fingerprintEnvelope)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO event_idempotency.*RETURNING received_at`).
		WillReturnRows(sqlmock.NewRows([]string{"received_at"}))
	mock.ExpectQuery(`(?s)SELECT i.payload_hash, i.received_at, o.id.*FROM event_idempotency i.*LEFT JOIN event_outbox o`).
		WithArgs(eventID).
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "received_at", "message_id"}).
			AddRow(payloadHash, now, messageID))
	mock.ExpectCommit()

	result, err := NewRealtimeRepositoryWithDB(db).AppendEvent(context.Background(), "workspace-1", envelope, now.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, result.Duplicate)
	assert.Equal(t, messageID, result.MessageID)
	assert.Equal(t, now, result.ReceivedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryClaimInboxReturnsCompletedDuplicateWithoutOwnership(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	messageID := uuid.New()
	existingToken := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)WITH claimed AS .*INSERT INTO consumer_inbox.*ON CONFLICT.*UNION ALL`).
		WillReturnRows(sqlmock.NewRows([]string{
			"consumer", "message_id", "status", "attempts", "claim_token",
			"claim_expires_at", "processed_at", "last_error", "created_at", "acquired",
		}).AddRow("rule", messageID, "completed", 1, existingToken, now, now, nil, now, false))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	claim, err := NewRealtimeRepositoryWithDB(db).ClaimInbox(
		context.Background(), tx, "workspace-1", "rule", messageID, now, time.Minute,
	)
	require.NoError(t, err)
	assert.Equal(t, domain.InboxStatusCompleted, claim.Status)
	assert.False(t, claim.Acquired)
	assert.Equal(t, existingToken, claim.ClaimToken)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryReserveSideEffectRejectsChangedRequest(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	execution := domain.SideEffectExecution{
		EffectKey:           "effect-1",
		ContactAutomationID: "journey-1",
		AutomationVersion:   2,
		NodeID:              "send-1",
		ExecutionVersion:    7,
		Channel:             "email",
		RequestHash:         "new-request",
	}
	mock.ExpectQuery(`(?s)INSERT INTO side_effect_executions.*ON CONFLICT \(effect_key\) DO NOTHING`).
		WillReturnRows(sqlmock.NewRows([]string{
			"effect_key", "contact_automation_id", "automation_version", "node_id",
			"execution_version", "channel", "status", "provider_message_id",
			"request_hash", "attempts", "last_error", "created_at", "updated_at",
		}))
	mock.ExpectQuery(`(?s)SELECT effect_key.*FROM side_effect_executions WHERE effect_key = \$1`).
		WithArgs("effect-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"effect_key", "contact_automation_id", "automation_version", "node_id",
			"execution_version", "channel", "status", "provider_message_id",
			"request_hash", "attempts", "last_error", "created_at", "updated_at",
		}).AddRow("effect-1", "journey-1", 2, "send-1", 7, "email", "reserved", nil,
			"original-request", 0, nil, time.Now(), time.Now()))

	_, _, err = NewRealtimeRepositoryWithDB(db).ReserveSideEffect(context.Background(), "workspace-1", execution)
	require.ErrorIs(t, err, domain.ErrSideEffectHashConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}
