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

func TestV45MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V45Migration{}
	assert.Equal(t, 45.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "47.0", config.VERSION)
	registered, ok := GetRegisteredMigration(45.0)
	require.True(t, ok)
	assert.IsType(t, &V45Migration{}, registered)
}

func TestV45UpdateWorkspaceAddsChannelSendLedgerAndSMSAddresses(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_channel_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_provider_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_platform_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_provider_platform_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_channel_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_provider_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_platform_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_provider_platform_check").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS channel_send_executions").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_channel_send_executions_message").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V45Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
