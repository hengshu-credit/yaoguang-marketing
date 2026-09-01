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

func TestV55MigrationCreatesAndSeedsTemplateCategories(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS template_categories").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO template_categories").WillReturnResult(sqlmock.NewResult(0, 9))
	mock.ExpectExec("INSERT INTO template_categories").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_template_categories_active_order").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V55Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV55MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V55Migration{}
	assert.Equal(t, 55.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "55.0", config.VERSION)
	registered, ok := GetRegisteredMigration(55.0)
	require.True(t, ok)
	assert.IsType(t, &V55Migration{}, registered)
}

func TestV55UpdateWorkspaceRequiresDatabase(t *testing.T) {
	err := (&V55Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, nil)
	assert.ErrorContains(t, err, "workspace database is required")
}
