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

func TestV48MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V48Migration{}

	assert.Equal(t, 48.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "55.0", config.VERSION)
	registered, ok := GetRegisteredMigration(48.0)
	require.True(t, ok)
	assert.IsType(t, &V48Migration{}, registered)
}

func TestV48UpdateWorkspaceCreatesLedgerAndMigratesLegacyDeliveries(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for _, statement := range schema.DeliveryTableDefinitions() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("INSERT INTO delivery_intents.*FROM channel_send_executions").
		WillReturnResult(sqlmock.NewResult(0, 7))
	mock.ExpectExec("INSERT INTO delivery_intents.*FROM email_queue").
		WillReturnResult(sqlmock.NewResult(0, 11))
	mock.ExpectExec("UPDATE email_queue.*SET delivery_intent_id = intent.id").
		WillReturnResult(sqlmock.NewResult(0, 11))

	err = (&V48Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-1"},
		db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV48LegacyMigrationPreservesUnresolvedIdentityAndSafeStatus(t *testing.T) {
	sql := v48MigrateChannelSendsSQL + "\n" + v48MigrateEmailQueueSQL

	assert.Contains(t, sql, "legacy_identity")
	assert.Contains(t, sql, "contact_email")
	assert.Contains(t, sql, "'submitting'")
	assert.Contains(t, sql, "'unknown'")
	assert.Contains(t, sql, "'transient_failed'")
	assert.Contains(t, sql, "sha256")
	assert.NotContains(t, sql, "md5(")
}

func TestV48UpdateWorkspaceReportsFailingWorkspace(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(schema.DeliveryTableDefinitions()[0])).
		WillReturnError(assert.AnError)

	err = (&V48Migration{}).UpdateWorkspace(
		context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-broken"}, db,
	)
	assert.ErrorContains(t, err, "workspace-broken")
	assert.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}
