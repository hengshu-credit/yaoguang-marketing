package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/migrations"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
)

func TestV40RealtimeMigrationIsIdempotentAndBridgesTimelineOnce(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	db, err := suite.DataFactory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ctx := context.Background()

	// A fresh workspace already has v40. Remove only v40 objects and columns to
	// reproduce the released v39 shape before exercising the compiled migration.
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS contact_timeline_realtime_bridge ON contact_timeline;
		DROP FUNCTION IF EXISTS notifuse_capture_timeline_event();
		DROP FUNCTION IF EXISTS automation_enroll_contact(VARCHAR(36), VARCHAR(255), VARCHAR(36), VARCHAR(20), UUID);
		DROP TABLE IF EXISTS side_effect_executions;
		DROP TABLE IF EXISTS automation_match_audit;
		DROP TABLE IF EXISTS automation_trigger_bindings;
		DROP TABLE IF EXISTS consumer_inbox;
		DROP TABLE IF EXISTS event_outbox;
		DROP TABLE IF EXISTS event_ledger CASCADE;
		DROP TABLE IF EXISTS event_idempotency;
		ALTER TABLE contact_automations
			DROP COLUMN IF EXISTS origin_event_id,
			DROP COLUMN IF EXISTS automation_version,
			DROP COLUMN IF EXISTS state_version,
			DROP COLUMN IF EXISTS claim_token,
			DROP COLUMN IF EXISTS claimed_by,
			DROP COLUMN IF EXISTS claimed_at,
			DROP COLUMN IF EXISTS claim_expires_at;
		ALTER TABLE contact_timeline DROP COLUMN IF EXISTS origin_event_id;
		ALTER TABLE automations DROP COLUMN IF EXISTS version;
	`)
	require.NoError(t, err)

	migration := &migrations.V40Migration{}
	for range 2 {
		tx, txErr := db.BeginTx(ctx, nil)
		require.NoError(t, txErr)
		require.NoError(t, migration.UpdateWorkspace(ctx, suite.Config, workspace, tx))
		require.NoError(t, tx.Commit())
	}

	eventID := uuid.New()
	timelineID := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO contact_timeline (
			id, origin_event_id, email, operation, entity_type, kind,
			entity_id, changes, created_at
		) VALUES ($1, $2, 'ada@example.com', 'update', 'contact',
			'contact.updated', 'ada@example.com', '{"language":{"new":"fr"}}'::jsonb, NOW())
	`, timelineID, eventID)
	require.NoError(t, err)

	for table, predicate := range map[string]string{
		"event_idempotency": "id = $1",
		"event_ledger":      "id = $1 AND timeline_id = $2",
		"event_outbox":      "event_id = $1",
	} {
		var count int
		args := []any{eventID}
		if table == "event_ledger" {
			args = append(args, timelineID)
		}
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE "+predicate, args...).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, table)
	}
}
