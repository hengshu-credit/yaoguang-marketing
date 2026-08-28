package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageHistoryRepository_GetBySMTPMessageID(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	ws := "ws-1"

	t.Run("found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), ws).Return(db, nil)
		rows := sqlmock.NewRows([]string{"id", "contact_email", "automation_id"}).
			AddRow("m1", "jane@x.com", "auto-1")
		mock.ExpectQuery("SELECT id, contact_email, automation_id FROM message_history").
			WithArgs("orig@x.com").
			WillReturnRows(rows)

		m, err := repo.GetBySMTPMessageID(ctx, ws, "orig@x.com")
		require.NoError(t, err)
		require.NotNil(t, m)
		assert.Equal(t, "m1", m.ID)
		assert.Equal(t, "jane@x.com", m.ContactEmail)
		require.NotNil(t, m.AutomationID)
		assert.Equal(t, "auto-1", *m.AutomationID)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), ws).Return(db, nil)
		mock.ExpectQuery("SELECT id, contact_email, automation_id FROM message_history").
			WithArgs("missing@x.com").
			WillReturnRows(sqlmock.NewRows([]string{"id", "contact_email", "automation_id"}))

		m, err := repo.GetBySMTPMessageID(ctx, ws, "missing@x.com")
		require.NoError(t, err)
		assert.Nil(t, m)
	})
}
