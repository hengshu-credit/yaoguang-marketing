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

func TestV41MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V41Migration{}
	assert.Equal(t, 41.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "45.0", config.VERSION)
	registered, ok := GetRegisteredMigration(41.0)
	require.True(t, ok)
	assert.IsType(t, &V41Migration{}, registered)
}

func TestV41UpdateWorkspaceInstallsAudienceIngestSchema(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS contact_profiles.*contact_tag_timeline_trigger").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V41Migration{}).UpdateWorkspace(
		context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
