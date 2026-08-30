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

func TestV47MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V47Migration{}

	assert.Equal(t, 47.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "47.0", config.VERSION)
	registered, ok := GetRegisteredMigration(47.0)
	require.True(t, ok)
	assert.IsType(t, &V47Migration{}, registered)
}

func TestV47UpdateWorkspaceBackfillsEveryLegacyMarketingReference(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for _, statement := range schema.CustomerAuthorityTableDefinitions() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	for _, table := range []string{
		"contact_lists",
		"contact_segments",
		"custom_events",
		"contact_timeline",
		"contact_automations",
		"automation_trigger_log",
		"message_history",
		"email_queue",
	} {
		mock.ExpectExec("UPDATE " + table + ".*SET customer_id = c.customer_id.*FROM contacts c").
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec("INSERT INTO customer_projection_reconciliation.*SELECT.*FROM " + table).
			WithArgs(table).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	err = (&V47Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-1"},
		db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV47UpdateWorkspaceReportsTheFailingWorkspace(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(schema.CustomerAuthorityTableDefinitions()[0])).
		WillReturnError(assert.AnError)

	err = (&V47Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-broken"},
		db,
	)
	assert.ErrorContains(t, err, "workspace-broken")
	assert.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}
