package database

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/postgresdriver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJourneyEnrollmentAuthorityOnPostgres(t *testing.T) {
	adminDSN := os.Getenv("JOURNEY_POSTGRES_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("JOURNEY_POSTGRES_ADMIN_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open(postgresdriver.Name, adminDSN)
	require.NoError(t, err)
	defer admin.Close()
	require.NoError(t, admin.PingContext(ctx))

	databaseName := "notifuse_journey_test_" + uuid.NewString()[:8]
	_, err = admin.ExecContext(ctx, `CREATE DATABASE "`+databaseName+`"`)
	require.NoError(t, err)
	defer func() {
		_, _ = admin.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, databaseName)
		_, _ = admin.ExecContext(ctx, `DROP DATABASE IF EXISTS "`+databaseName+`"`)
	}()

	workspaceURL, err := url.Parse(adminDSN)
	require.NoError(t, err)
	workspaceURL.Path = "/" + databaseName
	dsn := workspaceURL.String()
	db, err := sql.Open(postgresdriver.Name, dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, InitializeWorkspaceDatabase(db))

	customerID := uuid.New()
	_, err = db.ExecContext(ctx, `INSERT INTO customers (id, customer_no) VALUES ($1, $2)`, customerID, "U0001"+uuid.NewString()[:20])
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO contacts (email, customer_id) VALUES ('old@example.com', $1)`, customerID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO automations (id, workspace_id, name, status, trigger_config, root_node_id, version)
		VALUES ('automation-1', 'workspace-1', 'Journey integration', 'live', '{"event_kind":"contact.updated","frequency":"every_time"}', 'root', 7)`)
	require.NoError(t, err)

	callWithGuard := func(email, frequency string, eventID *uuid.UUID, guard string, version int) (string, string, *time.Time) {
		t.Helper()
		var outcome string
		var contactAutomationID sql.NullString
		var retryAt sql.NullTime
		err := db.QueryRowContext(ctx, `SELECT outcome, contact_automation_id, retry_at
			FROM automation_enroll_customer('automation-1', $1, $2, 'root', $3, $4, $5::jsonb, $6, 'realtime')`,
			customerID, email, frequency, eventID, guard, version).Scan(&outcome, &contactAutomationID, &retryAt)
		require.NoError(t, err)
		var retry *time.Time
		if retryAt.Valid {
			retry = &retryAt.Time
		}
		return outcome, contactAutomationID.String, retry
	}
	call := func(email, frequency string, eventID *uuid.UUID) (string, string, *time.Time) {
		return callWithGuard(email, frequency, eventID, `{"enabled":false}`, 7)
	}

	outcome, firstID, _ := call("old@example.com", "once", nil)
	assert.Equal(t, "enrolled", outcome)
	assert.NotEmpty(t, firstID)
	outcome, _, _ = call("new-primary@example.com", "once", nil)
	assert.Equal(t, "already_once", outcome, "primary Email changes must not bypass Customer-level once")

	eventA := uuid.New()
	outcome, _, _ = call("new-primary@example.com", "every_time", &eventA)
	assert.Equal(t, "enrolled", outcome)
	outcome, _, _ = call("new-primary@example.com", "every_time", &eventA)
	assert.Equal(t, "replayed_event", outcome)
	eventB := uuid.New()
	outcome, secondEventID, _ := call("new-primary@example.com", "every_time", &eventB)
	assert.Equal(t, "enrolled", outcome)
	assert.NotEmpty(t, secondEventID)
	assert.NotEqual(t, firstID, secondEventID)

	staleEvent := uuid.New()
	outcome, _, _ = callWithGuard("new-primary@example.com", "every_time", &staleEvent, `{"enabled":false}`, 6)
	assert.Equal(t, "guard_denied", outcome, "a stale Automation version must not create an instance")

	_, err = db.ExecContext(ctx, `UPDATE journey_instances SET status = 'completed', completed_at = NOW(), started_at = NOW() - INTERVAL '2 hours'`)
	require.NoError(t, err)
	cooldownFirst := uuid.New()
	outcome, _, _ = callWithGuard("new-primary@example.com", "every_time", &cooldownFirst, `{"enabled":true,"cooldown":3600000000000}`, 7)
	assert.Equal(t, "enrolled", outcome)
	cooldownSecond := uuid.New()
	outcome, _, retryAt := callWithGuard("new-primary@example.com", "every_time", &cooldownSecond, `{"enabled":true,"cooldown":3600000000000}`, 7)
	assert.Equal(t, "guard_deferred", outcome)
	assert.NotNil(t, retryAt)
	outcome, _, immediateRetryAt := callWithGuard("new-primary@example.com", "every_time", &cooldownSecond, `{"enabled":true,"cooldown":3600000000000}`, 7)
	assert.Equal(t, "guard_deferred", outcome, "an early transport retry must not bypass retry_at")
	assert.Equal(t, retryAt, immediateRetryAt)

	_, err = db.ExecContext(ctx, `UPDATE journey_instances SET status = 'completed', completed_at = NOW(), started_at = NOW() - INTERVAL '2 hours' WHERE status = 'active'`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE journey_entry_decisions SET retry_at = NOW() - INTERVAL '1 second'
		WHERE automation_id = 'automation-1' AND origin_event_id = $1 AND decision = 'guard_deferred'`, cooldownSecond)
	require.NoError(t, err)
	outcome, resumedID, _ := callWithGuard("new-primary@example.com", "every_time", &cooldownSecond, `{"enabled":true,"cooldown":3600000000000}`, 7)
	assert.Equal(t, "enrolled", outcome, "a due deferred reservation must be re-evaluated")
	assert.NotEmpty(t, resumedID)

	_, err = db.ExecContext(ctx, `UPDATE contact_automations SET status = 'completed', scheduled_at = NULL WHERE id = $1`, resumedID)
	require.NoError(t, err)
	var projectedStatus string
	var traceCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status FROM journey_instances WHERE contact_automation_id = $1`, resumedID).Scan(&projectedStatus))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journey_instance_events event
		JOIN journey_instances instance ON instance.id = event.journey_instance_id
		WHERE instance.contact_automation_id = $1 AND event.event_type = 'state_changed'`, resumedID).Scan(&traceCount))
	assert.Equal(t, "completed", projectedStatus)
	assert.Equal(t, 1, traceCount, "all execution paths must project state changes into the Journey trace")
}
