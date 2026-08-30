package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var deliveryIntentColumns = []string{
	"id", "effect_key", "request_hash", "source_type", "source_id", "source_version",
	"customer_id", "legacy_identity", "channel", "template_id", "template_version",
	"node_or_phase", "occurrence", "variant", "status", "suppression_reason",
	"metadata", "created_at", "updated_at",
}

func deliveryIntentRow(intent domain.DeliveryIntent) *sqlmock.Rows {
	metadata, _ := json.Marshal(intent.Metadata)
	return sqlmock.NewRows(deliveryIntentColumns).AddRow(
		intent.ID, intent.EffectKey, intent.RequestHash, intent.SourceType, intent.SourceID,
		intent.SourceVersion, testNullableString(intent.CustomerID), testNullableString(intent.LegacyIdentity),
		intent.Channel, testNullableString(intent.TemplateID), nullablePositiveInt64(intent.TemplateVersion),
		intent.NodeOrPhase, intent.Occurrence, intent.Variant, intent.Status,
		testNullableString(intent.SuppressionReason), metadata, intent.CreatedAt, intent.UpdatedAt,
	)
}

func testNullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func nullablePositiveInt64(value int64) interface{} {
	if value <= 0 {
		return nil
	}
	return value
}

func testDeliveryIntent() domain.DeliveryIntent {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	return domain.DeliveryIntent{
		ID: "11111111-1111-4111-8111-111111111111", EffectKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SourceType:  domain.DeliverySourceBroadcast, SourceID: "broadcast-1", SourceVersion: "3",
		CustomerID: "22222222-2222-4222-8222-222222222222", Channel: "email",
		TemplateID: "template-1", TemplateVersion: 4, NodeOrPhase: "primary",
		Occurrence: "recipient-42", Variant: "control", Status: domain.DeliveryStatusReserved,
		Metadata: domain.MapOfAny{"snapshot_ordinal": 42}, CreatedAt: now, UpdatedAt: now,
	}
}

func testDeliveryQueueEntry(intentID string) *domain.EmailQueueEntry {
	return &domain.EmailQueueEntry{
		ID: "queue-1", DeliveryIntentID: intentID, SourceType: domain.EmailQueueSourceBroadcast,
		SourceID: "broadcast-1", IntegrationID: "integration-1", ProviderKind: domain.EmailProviderKindSMTP,
		ContactEmail: "recipient@example.com", MessageID: "message-1", TemplateID: "template-1",
		Payload: domain.EmailQueuePayload{FromAddress: "sender@example.com", Subject: "Hello"},
	}
}

func TestDeliveryReserveAndEnqueueCreatesIntentQueueAndStatusAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intent := testDeliveryIntent()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO delivery_intents.*ON CONFLICT.*RETURNING").
		WillReturnRows(deliveryIntentRow(intent))
	mock.ExpectExec(`INSERT INTO email_queue.*delivery_intent_id.*ON CONFLICT \(delivery_intent_id\) DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE delivery_intents.*SET status = 'queued'.*WHERE id = \$1 AND status IN \('reserved', 'transient_failed'\)`).
		WithArgs(intent.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.ReserveAndEnqueue(context.Background(), "workspace-1", intent, testDeliveryQueueEntry(intent.ID))
	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.True(t, result.QueueCreated)
	assert.Equal(t, domain.DeliveryStatusQueued, result.Intent.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryReserveAndEnqueueReturnsExistingIntentForSameHash(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intent := testDeliveryIntent()
	existing := intent
	existing.Status = domain.DeliveryStatusQueued

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO delivery_intents.*ON CONFLICT.*RETURNING").
		WillReturnRows(sqlmock.NewRows(deliveryIntentColumns))
	mock.ExpectQuery(`SELECT .* FROM delivery_intents WHERE effect_key = \$1 FOR UPDATE`).
		WithArgs(intent.EffectKey).WillReturnRows(deliveryIntentRow(existing))
	mock.ExpectCommit()

	result, err := repo.ReserveAndEnqueue(context.Background(), "workspace-1", intent, testDeliveryQueueEntry(intent.ID))
	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.False(t, result.QueueCreated)
	assert.Equal(t, existing.ID, result.Intent.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryReserveAndEnqueueRepairsReservedIntentWithoutQueue(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intent := testDeliveryIntent()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO delivery_intents.*ON CONFLICT.*RETURNING").
		WillReturnRows(sqlmock.NewRows(deliveryIntentColumns))
	mock.ExpectQuery(`SELECT .* FROM delivery_intents WHERE effect_key = \$1 FOR UPDATE`).
		WithArgs(intent.EffectKey).WillReturnRows(deliveryIntentRow(intent))
	mock.ExpectExec(`INSERT INTO email_queue.*ON CONFLICT \(delivery_intent_id\) DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE delivery_intents.*status = 'queued'`).
		WithArgs(intent.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.ReserveAndEnqueue(context.Background(), "workspace-1", intent, testDeliveryQueueEntry("wrong-id"))
	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.True(t, result.QueueCreated)
	assert.Equal(t, domain.DeliveryStatusQueued, result.Intent.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryReserveAndEnqueueRejectsSameEffectKeyWithDifferentHash(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intent := testDeliveryIntent()
	existing := intent
	existing.RequestHash = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO delivery_intents.*ON CONFLICT.*RETURNING").
		WillReturnRows(sqlmock.NewRows(deliveryIntentColumns))
	mock.ExpectQuery(`SELECT .* FROM delivery_intents WHERE effect_key = \$1 FOR UPDATE`).
		WithArgs(intent.EffectKey).WillReturnRows(deliveryIntentRow(existing))
	mock.ExpectRollback()

	result, err := repo.ReserveAndEnqueue(context.Background(), "workspace-1", intent, testDeliveryQueueEntry(intent.ID))
	assert.ErrorIs(t, err, domain.ErrDeliveryIntentHashConflict)
	assert.False(t, result.Created)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryReserveAndEnqueueRollsBackWhenQueueInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intent := testDeliveryIntent()
	queueErr := errors.New("queue unavailable")

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO delivery_intents.*ON CONFLICT.*RETURNING").
		WillReturnRows(deliveryIntentRow(intent))
	mock.ExpectExec("INSERT INTO email_queue").WillReturnError(queueErr)
	mock.ExpectRollback()

	_, err = repo.ReserveAndEnqueue(context.Background(), "workspace-1", intent, testDeliveryQueueEntry(intent.ID))
	assert.ErrorIs(t, err, queueErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryRepositoryResolvesAuthorityCustomerID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)

	mock.ExpectQuery("SELECT customer_id::text FROM contacts").
		WithArgs("recipient@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"customer_id"}).
			AddRow("22222222-2222-4222-8222-222222222222"))

	customerID, err := repo.ResolveCustomerID(context.Background(), "workspace-1", "recipient@example.com")
	require.NoError(t, err)
	assert.Equal(t, "22222222-2222-4222-8222-222222222222", customerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryRepositoryStartAttemptPersistsSubmittingBeforeProviderCall(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intent := testDeliveryIntent()
	intent.Status = domain.DeliveryStatusQueued
	claimToken := "33333333-3333-4333-8333-333333333333"
	lease := time.Now().UTC().Add(time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT effect_key, request_hash, status.*delivery_intents.*FOR UPDATE`).
		WithArgs(intent.ID).WillReturnRows(sqlmock.NewRows([]string{"effect_key", "request_hash", "status"}).
		AddRow(intent.EffectKey, intent.RequestHash, intent.Status))
	mock.ExpectQuery(`SELECT EXISTS.*delivery_attempts`).WithArgs(intent.ID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt_no\), 0\) \+ 1`).WithArgs(intent.ID).
		WillReturnRows(sqlmock.NewRows([]string{"attempt_no"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO delivery_attempts`).
		WithArgs(sqlmock.AnyArg(), intent.ID, 1, "smtp", intent.RequestHash, claimToken, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE delivery_intents SET status = 'submitting'`).
		WithArgs(intent.ID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	attempt, err := repo.StartAttempt(context.Background(), "workspace-1", domain.DeliveryAttemptStart{
		IntentID: intent.ID, Provider: "smtp", ClaimToken: claimToken, LeaseExpiresAt: lease,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DeliveryStatusSubmitting, attempt.Status)
	assert.Equal(t, intent.EffectKey, attempt.EffectKey)
	assert.Equal(t, 1, attempt.AttemptNo)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryRepositoryUnknownOutcomeBlocksRetryAndCreatesReconciliation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intentID := "11111111-1111-4111-8111-111111111111"
	attemptID := "44444444-4444-4444-8444-444444444444"
	claimToken := "33333333-3333-4333-8333-333333333333"

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT intent_id, status FROM delivery_attempts`).WithArgs(attemptID, claimToken).
		WillReturnRows(sqlmock.NewRows([]string{"intent_id", "status"}).AddRow(intentID, domain.DeliveryStatusSubmitting))
	mock.ExpectQuery(`SELECT status FROM delivery_intents`).WithArgs(intentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(domain.DeliveryStatusSubmitting))
	mock.ExpectExec(`UPDATE delivery_attempts SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE delivery_intents SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE email_queue SET status = 'failed'.*next_retry_at = NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO delivery_reconciliations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.RecordAttemptOutcome(context.Background(), "workspace-1", attemptID, claimToken, domain.DeliveryAttemptOutcome{
		Status: domain.DeliveryStatusUnknown, ErrorCategory: "transport",
		ErrorDetail: "connection reset after submit", OccurredAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryRepositoryConfirmedOutcomeCompletesQueueWithoutDeleting(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewDeliveryRepositoryWithDB(db)
	intentID := "11111111-1111-4111-8111-111111111111"
	attemptID := "44444444-4444-4444-8444-444444444444"
	claimToken := "33333333-3333-4333-8333-333333333333"

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT intent_id, status FROM delivery_attempts`).WithArgs(attemptID, claimToken).
		WillReturnRows(sqlmock.NewRows([]string{"intent_id", "status"}).AddRow(intentID, domain.DeliveryStatusProviderAccepted))
	mock.ExpectQuery(`SELECT status FROM delivery_intents`).WithArgs(intentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(domain.DeliveryStatusProviderAccepted))
	mock.ExpectExec(`UPDATE delivery_attempts SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE delivery_intents SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE email_queue SET status = 'confirmed'.*completed_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.RecordAttemptOutcome(context.Background(), "workspace-1", attemptID, claimToken, domain.DeliveryAttemptOutcome{
		Status: domain.DeliveryStatusConfirmed, ProviderMessageID: "provider-1", OccurredAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
