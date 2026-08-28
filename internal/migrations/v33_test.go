package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV33Migration_Metadata(t *testing.T) {
	m := &V33Migration{}
	assert.Equal(t, 33.0, m.GetMajorVersion())
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV33Migration_UpdateSystem_NoOp(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	err = (&V33Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV33Migration_UpdateWorkspace_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("ALTER TABLE message_history ADD COLUMN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE automations ADD COLUMN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("idx_message_history_smtp_message_id").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("inbound_webhook_events_reply_dedup_idx").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V33Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws"}, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV33Migration_UpdateWorkspace_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("ALTER TABLE message_history ADD COLUMN").WillReturnError(errors.New("boom"))

	err = (&V33Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws"}, db)
	assert.Error(t, err)
}
