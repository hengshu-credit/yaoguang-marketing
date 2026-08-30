package migrations

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV49MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V49Migration{}
	assert.Equal(t, 49.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "50.0", config.VERSION)
	registered, ok := GetRegisteredMigration(49.0)
	require.True(t, ok)
	assert.IsType(t, &V49Migration{}, registered)
}

func TestV49UpdateWorkspaceCreatesMarketingSchemaAndCompatibilityAudience(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	for _, statement := range schema.MarketingTableDefinitions() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("INSERT INTO audiences.*FROM broadcasts").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO audience_versions.*JOIN broadcasts").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE audiences SET active_version = 1").WillReturnResult(sqlmock.NewResult(0, 2))
	err = (&V49Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV49CompatibilityKeepsHistoricalBroadcastJSONUntouched(t *testing.T) {
	sql := v49MigrateBroadcastAudiencesSQL + v49MigrateBroadcastAudienceVersionsSQL + v49ActivateBroadcastAudiencesSQL
	assert.NotContains(t, sql, "UPDATE broadcasts")
	assert.Contains(t, sql, "ON CONFLICT")
}
