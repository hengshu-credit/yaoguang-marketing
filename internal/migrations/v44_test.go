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

func TestV44MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V44Migration{}
	assert.Equal(t, 44.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "46.0", config.VERSION)
	registered, ok := GetRegisteredMigration(44.0)
	require.True(t, ok)
	assert.IsType(t, &V44Migration{}, registered)
}

func TestV44UpdateWorkspaceAddsDeliveryReceiptLedger(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS delivery_receipts").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_delivery_receipts_provider_message").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_delivery_receipts_message").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_external_id").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V44Migration{}).UpdateWorkspace(
		context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
