package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
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

func TestRealtimeRepositorySummarizesExactEventShadowDecisions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	from := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	mock.ExpectQuery(`(?s)WITH realtime AS.*FULL OUTER JOIN legacy.*FROM paired`).
		WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{
			"realtime_evaluated", "legacy_matched", "realtime_matched", "agreements",
			"decision_mismatches", "missing_realtime", "realtime_only_matched",
		}).AddRow(100_000, 40_000, 40_001, 99_999, 1, 0, 1))

	summary, err := NewRealtimeRepositoryWithDB(db).SummarizeMatchAudits(
		context.Background(), "workspace-1", from, to,
	)

	require.NoError(t, err)
	assert.Equal(t, int64(100_000), summary.RealtimeEvaluated)
	assert.Equal(t, int64(1), summary.DecisionMismatches)
	assert.InDelta(t, 0.99999, summary.ConsistencyRate, 0.0000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositorySideEffectTransitionUsesExpectedStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(`(?s)UPDATE side_effect_executions.*WHERE effect_key = \$1 AND status = \$2`).
		WithArgs("effect-1", domain.SideEffectStatusReserved, domain.SideEffectStatusSubmitted, nil, now).
		WillReturnResult(sqlmock.NewResult(0, 1))

	transitioned, err := NewRealtimeRepositoryWithDB(db).TransitionSideEffect(
		context.Background(), "workspace-1", "effect-1",
		domain.SideEffectStatusReserved, domain.SideEffectStatusSubmitted, now, nil,
	)

	require.NoError(t, err)
	assert.True(t, transitioned)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryRejectsUnsafeSideEffectTransition(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	_, err = NewRealtimeRepositoryWithDB(db).TransitionSideEffect(
		context.Background(), "workspace-1", "effect-1",
		domain.SideEffectStatusUnknown, domain.SideEffectStatusSubmitted, time.Now(), nil,
	)

	require.ErrorContains(t, err, "invalid side effect transition")
}

func TestRealtimeRepositoryListsOnlyDependencyCandidates(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().UTC()
	compiled := []byte(`{"query":"SELECT TRUE","arguments":[],"root_node_id":"root","frequency":"every_time"}`)
	mock.ExpectQuery(`(?s)FROM automation_trigger_bindings b.*JOIN automations a.*dependency_keys && \$3::text\[\]`).
		WithArgs("contact.updated", "contact", pq.Array([]string{"changes.language"})).
		WillReturnRows(sqlmock.NewRows([]string{
			"automation_id", "automation_version", "event_type", "subject_type",
			"dependency_keys", "condition_hash", "compiled_condition", "created_at",
		}).AddRow("automation-1", 2, "contact.updated", "contact",
			"{changes.language}", "hash-1", compiled, now))

	bindings, err := NewRealtimeRepositoryWithDB(db).ListTriggerBindings(
		context.Background(), "workspace-1", "contact.updated", "contact", []string{"changes.language"},
	)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, 2, bindings[0].AutomationVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryReplacesBindingsTransactionally(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	binding := domain.TriggerBinding{
		AutomationID: "automation-1", AutomationVersion: 3,
		EventType: "contact.updated", SubjectType: "contact",
		DependencyKeys: []string{"changes.language"}, ConditionHash: "hash-1",
		CompiledCondition: json.RawMessage(`{"query":"SELECT TRUE"}`),
	}
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM automation_trigger_bindings`).
		WithArgs("automation-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO automation_trigger_bindings`).
		WithArgs("automation-1", 3, "contact.updated", "contact",
			pq.Array([]string{"changes.language"}), "hash-1", []byte(binding.CompiledCondition)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewRealtimeRepositoryWithDB(db).ReplaceTriggerBindings(
		context.Background(), "workspace-1", "automation-1", []domain.TriggerBinding{binding},
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryProcessesShadowRuleInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	request := ruleProcessFixture(false)
	claimToken := uuid.New()
	compiled := []byte(`{"query":"SELECT TRUE","arguments":[],"root_node_id":"root","frequency":"every_time"}`)
	mock.ExpectBegin()
	expectInboxClaim(mock, request, claimToken, true, "processing")
	mock.ExpectQuery(`(?s)FROM automation_trigger_bindings b.*JOIN automations a`).
		WillReturnRows(sqlmock.NewRows([]string{
			"automation_id", "automation_version", "event_type", "subject_type",
			"dependency_keys", "condition_hash", "compiled_condition", "created_at",
		}).AddRow("automation-1", 2, "contact.updated", "contact", "{}", "hash-1", compiled, request.Now))
	mock.ExpectQuery(`SELECT TRUE`).WillReturnRows(sqlmock.NewRows([]string{"matched"}).AddRow(true))
	mock.ExpectQuery(`(?s)INSERT INTO automation_match_audit.*RETURNING decision_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"decision_hash"}).AddRow(ruleDecisionHash("hash-1", true)))
	mock.ExpectExec(`(?s)UPDATE consumer_inbox.*status = 'completed'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewRealtimeRepositoryWithDB(db).ProcessRuleEvent(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, domain.RuleProcessResult{Candidates: 1, Matched: 1}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryShadowOnceDecisionMatchesLegacyDedupGate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	request := ruleProcessFixture(false)
	claimToken := uuid.New()
	compiled := []byte(`{"query":"SELECT TRUE","arguments":[],"root_node_id":"root","frequency":"once"}`)
	mock.ExpectBegin()
	expectInboxClaim(mock, request, claimToken, true, "processing")
	mock.ExpectQuery(`(?s)FROM automation_trigger_bindings b.*JOIN automations a`).
		WillReturnRows(sqlmock.NewRows([]string{
			"automation_id", "automation_version", "event_type", "subject_type",
			"dependency_keys", "condition_hash", "compiled_condition", "created_at",
		}).AddRow("automation-1", 2, "contact.updated", "contact", "{}", "hash-1", compiled, request.Now))
	mock.ExpectQuery(`SELECT TRUE`).WillReturnRows(sqlmock.NewRows([]string{"matched"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT NOT EXISTS.*automation_trigger_log`).
		WithArgs("automation-1", request.Envelope.Subject.ContactEmail).
		WillReturnRows(sqlmock.NewRows([]string{"eligible"}).AddRow(false))
	mock.ExpectQuery(`(?s)INSERT INTO automation_match_audit.*RETURNING decision_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"decision_hash"}).AddRow(ruleDecisionHash("hash-1", false)))
	mock.ExpectExec(`(?s)UPDATE consumer_inbox.*status = 'completed'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewRealtimeRepositoryWithDB(db).ProcessRuleEvent(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, domain.RuleProcessResult{Candidates: 1}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryPrimaryRuleEnrollsAndQueuesJourney(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	request := ruleProcessFixture(true)
	claimToken := uuid.New()
	compiled := []byte(`{"query":"SELECT TRUE","arguments":[],"root_node_id":"root","frequency":"every_time"}`)
	mock.ExpectBegin()
	expectInboxClaim(mock, request, claimToken, true, "processing")
	mock.ExpectQuery(`(?s)FROM automation_trigger_bindings b.*JOIN automations a`).
		WillReturnRows(sqlmock.NewRows([]string{
			"automation_id", "automation_version", "event_type", "subject_type",
			"dependency_keys", "condition_hash", "compiled_condition", "created_at",
		}).AddRow("automation-1", 2, "contact.updated", "contact", "{}", "hash-1", compiled, request.Now))
	mock.ExpectQuery(`SELECT TRUE`).WillReturnRows(sqlmock.NewRows([]string{"matched"}).AddRow(true))
	mock.ExpectQuery(`(?s)WITH live AS .*INSERT INTO contact_automations`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("journey-1"))
	mock.ExpectExec(`(?s)UPDATE automations.*stats = jsonb_set`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO automation_node_executions`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO event_outbox.*notifuse.jobs`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO automation_match_audit.*RETURNING decision_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"decision_hash"}).AddRow(ruleDecisionHash("hash-1", true)))
	mock.ExpectExec(`(?s)UPDATE consumer_inbox.*status = 'completed'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewRealtimeRepositoryWithDB(db).ProcessRuleEvent(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, domain.RuleProcessResult{Candidates: 1, Matched: 1, Enrolled: 1}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRealtimeRepositoryCompletedRuleMessageDoesNotEnrollTwice(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	request := ruleProcessFixture(true)
	claimToken := uuid.New()
	mock.ExpectBegin()
	expectInboxClaim(mock, request, claimToken, false, "completed")
	mock.ExpectRollback()

	result, err := NewRealtimeRepositoryWithDB(db).ProcessRuleEvent(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.Duplicate)
	assert.Zero(t, result.Enrolled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func ruleProcessFixture(primary bool) domain.RuleProcessRequest {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	messageID := uuid.New()
	return domain.RuleProcessRequest{
		WorkspaceID: "workspace-1", Consumer: "rule-worker", MessageID: messageID,
		Engine: domain.MatchEngineRealtime, Primary: primary, Now: now, InboxLease: time.Minute,
		DependencyKeys: []string{"changes.language"},
		Envelope: domain.EventEnvelope{
			ID: messageID, EventID: uuid.New(), Type: "contact.updated", SchemaVersion: 1,
			WorkspaceID: "workspace-1",
			Subject:     domain.EventSubject{Type: "contact", ID: "person@example.com", ContactEmail: "person@example.com"},
			Source:      "contact_timeline", OccurredAt: now, ReceivedAt: now,
			CorrelationID: uuid.New(), Data: json.RawMessage(`{"changes":{"language":{"new":"fr"}}}`),
		},
	}
}

func expectInboxClaim(
	mock sqlmock.Sqlmock,
	request domain.RuleProcessRequest,
	claimToken uuid.UUID,
	acquired bool,
	status string,
) {
	mock.ExpectQuery(`(?s)WITH claimed AS .*INSERT INTO consumer_inbox.*ON CONFLICT.*UNION ALL`).
		WillReturnRows(sqlmock.NewRows([]string{
			"consumer", "message_id", "status", "attempts", "claim_token",
			"claim_expires_at", "processed_at", "last_error", "created_at", "acquired",
		}).AddRow(request.Consumer, request.MessageID, status, 1, claimToken,
			request.Now.Add(request.InboxLease), nil, nil, request.Now, acquired))
}
