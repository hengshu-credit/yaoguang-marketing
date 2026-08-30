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

func TestV43MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V43Migration{}
	assert.Equal(t, 43.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "51.0", config.VERSION)
	registered, ok := GetRegisteredMigration(43.0)
	require.True(t, ok)
	assert.IsType(t, &V43Migration{}, registered)
}

func TestV43UpdateWorkspaceAddsMessageTemplateColumns(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS sms JSONB").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS push JSONB").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V43Migration{}).UpdateWorkspace(
		context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
