package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV54MigrationAddsGenericTemplateContentAndWebhookNonceLedger(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS content JSONB").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS content_schema_version INTEGER").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS channel_webhook_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_channel_webhook_nonces_expiry").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V54Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV54MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V54Migration{}
	assert.Equal(t, 54.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "54.0", config.VERSION)
	registered, ok := GetRegisteredMigration(54.0)
	require.True(t, ok)
	assert.IsType(t, &V54Migration{}, registered)
}

func TestV54UpdateWorkspaceRequiresDatabase(t *testing.T) {
	err := (&V54Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, nil)
	assert.ErrorContains(t, err, "workspace database is required")
}
