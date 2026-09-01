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

func TestV53MigrationAddsResolvedAudienceSourceToCampaignRuns(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_id").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_version").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_build_id").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("campaign_runs_audience_version_fkey").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_campaign_runs_audience_build").WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V53Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV53MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V53Migration{}
	assert.Equal(t, 53.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.Equal(t, "55.0", config.VERSION)
}
