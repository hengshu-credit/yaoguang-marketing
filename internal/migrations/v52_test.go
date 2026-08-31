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

func TestV52MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V52Migration{}
	assert.Equal(t, 52.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "54.0", config.VERSION)

	registered := GetRegisteredMigrations()
	found := false
	for _, registeredMigration := range registered {
		if registeredMigration.GetMajorVersion() == migration.GetMajorVersion() {
			found = true
			break
		}
	}
	assert.True(t, found, "v52 migration should be registered")
}

func TestV52MigrationAddsListCampaignSourceAndImportBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("ALTER TABLE campaign_versions ALTER COLUMN audience_id DROP NOT NULL").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE campaign_versions ALTER COLUMN audience_version DROP NOT NULL").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE campaign_versions ADD COLUMN IF NOT EXISTS list_id").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE campaign_versions DROP CONSTRAINT IF EXISTS campaign_versions_source_check").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE campaign_versions ADD CONSTRAINT campaign_versions_source_check").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE import_jobs ADD COLUMN IF NOT EXISTS list_ids").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V52Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-1"},
		db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV52UpdateWorkspaceRequiresDatabase(t *testing.T) {
	err := (&V52Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-1"},
		nil,
	)
	assert.ErrorContains(t, err, "workspace database is required")
}
