package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV40MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V40Migration{}

	assert.Equal(t, 40.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "40.0", config.VERSION)

	registered, ok := GetRegisteredMigration(40.0)
	require.True(t, ok)
	assert.IsType(t, &V40Migration{}, registered)
}

func TestV40WorkspaceMigrationDefinesRealtimeTables(t *testing.T) {
	sql := (&V40Migration{}).workspaceSQL("workspace-123")

	for _, table := range []string{
		"event_idempotency",
		"event_ledger",
		"event_outbox",
		"consumer_inbox",
		"automation_trigger_bindings",
		"automation_match_audit",
		"side_effect_executions",
	} {
		assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS "+table)
	}
	assert.Contains(t, sql, "PARTITION BY RANGE (received_at)")
	assert.Contains(t, sql, "PRIMARY KEY (received_at, id)")
	assert.Contains(t, sql, "ADD COLUMN IF NOT EXISTS origin_event_id UUID")
	assert.Contains(t, sql, "notifuse_capture_timeline_event")
	assert.Contains(t, sql, "contact_timeline_realtime_bridge")
	assert.NotContains(t, sql, "timeline_id UUID UNIQUE",
		"a partitioned table cannot enforce a unique key that omits its partition key")
}

func TestV40WorkspaceMigrationBootstrapsFourMonthlyPartitions(t *testing.T) {
	sql := (&V40Migration{}).workspaceSQL("workspace-123")

	assert.Equal(t, 4, strings.Count(sql, "PARTITION OF event_ledger"))
}

func TestV40UpdateWorkspaceInstallsSchemaAndChecksInstalledTriggers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS event_idempotency.*contact_timeline_realtime_bridge").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("(?s)SELECT a.id, a.root_node_id, a.trigger_config").
		WillReturnRows(sqlmock.NewRows([]string{"id", "root_node_id", "trigger_config", "trigger_def"}))

	err = (&V40Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-123"},
		db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
