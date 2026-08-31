package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelWebhookNonceRepositoryReservesOnceAcrossProcesses(t *testing.T) {
	db, mockSQL, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	workspaceRepo := new(MockWorkspaceRepository)
	workspaceRepo.On("GetConnection", context.Background(), "ws-1").Return(db, nil).Twice()
	repo := NewChannelWebhookNonceRepository(workspaceRepo)
	expiresAt := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC)

	mockSQL.ExpectQuery("INSERT INTO channel_webhook_nonces").WithArgs("bridge-1", "nonce-1", expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"reserved"}).AddRow(true))
	reserved, err := repo.Reserve(context.Background(), "ws-1", "bridge-1", "nonce-1", expiresAt)
	require.NoError(t, err)
	assert.True(t, reserved)

	mockSQL.ExpectQuery("INSERT INTO channel_webhook_nonces").WithArgs("bridge-1", "nonce-1", expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"reserved"}).AddRow(false))
	reserved, err = repo.Reserve(context.Background(), "ws-1", "bridge-1", "nonce-1", expiresAt)
	require.NoError(t, err)
	assert.False(t, reserved)
	require.NoError(t, mockSQL.ExpectationsWereMet())
}
