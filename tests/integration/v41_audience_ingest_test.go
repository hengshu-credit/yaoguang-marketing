package integration

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV41AudienceProfileAndTagsBridgeToRealtimeLedger(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	workspaceDB, err := suite.DataFactory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	repo := repository.NewAudienceProfileRepository(suite.ServerManager.GetApp().GetWorkspaceRepository())
	ctx := context.Background()

	require.NoError(t, repo.EnsureContacts(ctx, workspace.ID, []string{"user@example.com"}))
	status := "active"
	require.NoError(t, repo.UpsertProfile(ctx, workspace.ID, "user@example.com", &status, map[string]interface{}{"plan": "pro"}))
	tags, err := repo.ApplyTags(ctx, workspace.ID, "user@example.com", domain.TagOperationSet, []string{"paid", "beta"})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta", "paid"}, tags)
	tags, err = repo.ApplyTags(ctx, workspace.ID, "user@example.com", domain.TagOperationRemove, []string{"beta"})
	require.NoError(t, err)
	assert.Equal(t, []string{"paid"}, tags)

	var storedStatus, plan string
	require.NoError(t, workspaceDB.QueryRowContext(ctx, `
		SELECT status, attributes->>'plan' FROM contact_profiles WHERE email = 'user@example.com'
	`).Scan(&storedStatus, &plan))
	assert.Equal(t, "active", storedStatus)
	assert.Equal(t, "pro", plan)

	for _, eventType := range []string{"contact.profile_created", "contact.tagged", "contact.untagged"} {
		var timelineCount, ledgerCount, outboxCount int
		require.NoError(t, workspaceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_timeline WHERE kind = $1`, eventType).Scan(&timelineCount))
		require.NoError(t, workspaceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_ledger WHERE event_type = $1`, eventType).Scan(&ledgerCount))
		require.NoError(t, workspaceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_outbox WHERE routing_key = $1`, eventType).Scan(&outboxCount))
		assert.Positive(t, timelineCount, eventType)
		assert.Equal(t, timelineCount, ledgerCount, eventType)
		assert.Equal(t, timelineCount, outboxCount, eventType)
	}
}
