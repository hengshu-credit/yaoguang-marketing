package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailQueueClaimPendingUsesOneAtomicLeaseStatement(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewEmailQueueRepositoryWithDB(db)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(domain.EmailQueuePayload{FromAddress: "sender@example.com"})
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{
		"id", "status", "priority", "source_type", "source_id", "integration_id", "provider_kind",
		"contact_email", "message_id", "template_id", "payload", "attempts", "max_attempts",
		"last_error", "next_retry_at", "created_at", "updated_at", "processed_at",
		"delivery_intent_id", "claim_token", "lease_expires_at", "completed_at",
	}).AddRow(
		"queue-1", "processing", 5, "broadcast", "broadcast-1", "integration-1", "smtp",
		"recipient@example.com", "message-1", "template-1", payload, 1, 3,
		nil, nil, now, now, nil,
		"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", now.Add(time.Minute), nil,
	)

	mock.ExpectQuery(`(?s)WITH candidates AS .*FOR UPDATE SKIP LOCKED.*UPDATE email_queue.*SET status = 'processing'.*claim_token = \$2.*lease_expires_at = \$3.*RETURNING`).
		WithArgs(2, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(rows)

	entries, err := repo.ClaimPending(context.Background(), "workspace-1", 2, time.Minute)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "queue-1", entries[0].ID)
	assert.NotEmpty(t, entries[0].ClaimToken)
	assert.NotNil(t, entries[0].LeaseExpiresAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEmailQueueClaimPendingExcludesUncertainProviderAttempts(t *testing.T) {
	assert.Contains(t, claimPendingEmailQueueSQL, "delivery_intents")
	assert.Contains(t, claimPendingEmailQueueSQL, "'reserved', 'queued', 'transient_failed'")
	assert.Contains(t, claimPendingEmailQueueSQL, "delivery_attempts")
	assert.Contains(t, claimPendingEmailQueueSQL, "'submitting', 'provider_accepted', 'unknown'")
	assert.NotContains(t, claimPendingEmailQueueSQL, "updated_at < NOW() - INTERVAL")
}

func TestEmailQueueCompleteClaimRequiresMatchingToken(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewEmailQueueRepositoryWithDB(db)
	completedAt := time.Date(2026, 8, 30, 12, 1, 0, 0, time.UTC)

	mock.ExpectExec(`UPDATE email_queue.*SET status = 'confirmed'.*claim_token = NULL.*WHERE id = \$1 AND claim_token = \$2`).
		WithArgs("queue-1", "claim-1", completedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.CompleteClaim(context.Background(), "workspace-1", "queue-1", "claim-1", completedAt)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEmailQueueCompleteClaimRejectsLostLease(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewEmailQueueRepositoryWithDB(db)
	completedAt := time.Date(2026, 8, 30, 12, 1, 0, 0, time.UTC)

	mock.ExpectExec(`UPDATE email_queue`).
		WithArgs("queue-1", "stale-claim", completedAt).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.CompleteClaim(context.Background(), "workspace-1", "queue-1", "stale-claim", completedAt)
	assert.ErrorContains(t, err, "claim was lost")
	require.NoError(t, mock.ExpectationsWereMet())
}
