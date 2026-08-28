package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationRepository_ExitContactJourneysOnReply(t *testing.T) {
	// The regexes assert the load-bearing predicates (not just the table name): a mutation
	// dropping status='active', exit_on_reply=TRUE, deleted_at IS NULL, or the entered_at
	// bound must fail this test rather than passing on a substring match.
	before := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	t.Run("with automation id scopes to that automation and asserts predicates", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		autoID := "auto-1"
		mock.ExpectExec(`(?s)UPDATE contact_automations SET status = 'exited', scheduled_at = NULL.*WHERE contact_email = \$2.*status = 'active'.*entered_at < \$3.*exit_on_reply = TRUE AND deleted_at IS NULL.*automation_id = \$4`).
			WithArgs("replied", "jane@x.com", sqlmock.AnyArg(), autoID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		n, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", &autoID, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("without automation id exits all flagged journeys, still predicate-scoped", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`(?s)UPDATE contact_automations SET status = 'exited', scheduled_at = NULL.*WHERE contact_email = \$2.*status = 'active'.*entered_at < \$3.*exit_on_reply = TRUE AND deleted_at IS NULL`).
			WithArgs("replied", "jane@x.com", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 2))

		n, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", nil, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
