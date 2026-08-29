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

func TestV42MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V42Migration{}
	assert.Equal(t, 42.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "45.0", config.VERSION)
	registered, ok := GetRegisteredMigration(42.0)
	require.True(t, ok)
	assert.IsType(t, &V42Migration{}, registered)
}

func TestV42UpdateWorkspaceInstallsContactEndpoints(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS contact_endpoints.*contact_endpoint_timeline_trigger").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V42Migration{}).UpdateWorkspace(
		context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
