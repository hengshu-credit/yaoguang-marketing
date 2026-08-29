package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

func TestRealtimeJourneyConcurrency(t *testing.T) {
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

	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO automations (
			id, workspace_id, name, status, trigger_config, root_node_id, nodes, stats, version
		) VALUES (
			'automation-lease-test', $1, 'Lease test', 'live',
			'{"event_kind":"contact.created","frequency":"every_time"}'::jsonb,
			'root', '[{"id":"root","type":"trigger"}]'::jsonb, '{}'::jsonb, 1
		);
		INSERT INTO contact_automations (
			id, automation_id, contact_email, current_node_id, status,
			entered_at, scheduled_at, context, automation_version, state_version
		) VALUES (
			'ca-lease-test', 'automation-lease-test', 'lease@example.com', 'root', 'active',
			$2, $2, '{}'::jsonb, 1, 0
		)
	`, workspace.ID, now)
	require.NoError(t, err)

	queryBuilder := service.NewQueryBuilder()
	automationRepo := repository.NewAutomationRepositoryWithDB(
		db, service.NewAutomationTriggerGenerator(queryBuilder),
	)

	start := make(chan struct{})
	claims := make(chan *domain.ContactAutomationClaim, 10)
	errors := make(chan error, 10)
	var workers sync.WaitGroup
	for worker := range 10 {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			<-start
			claim, acquired, claimErr := automationRepo.ClaimContactAutomation(
				context.Background(), workspace.ID, "ca-lease-test",
				fmt.Sprintf("journey-worker-%d", worker), now, time.Minute,
			)
			if claimErr != nil {
				errors <- claimErr
				return
			}
			if acquired {
				claims <- claim
			}
		}(worker)
	}
	close(start)
	workers.Wait()
	close(claims)
	close(errors)
	for claimErr := range errors {
		require.NoError(t, claimErr)
	}
	var winners []*domain.ContactAutomationClaim
	for claim := range claims {
		winners = append(winners, claim)
	}
	require.Len(t, winners, 1, "exactly one worker must own the journey lease")

	commit := domain.JourneyStateCommit{ContactAutomation: winners[0].ContactAutomation}
	commit.ContactAutomation.CurrentNodeID = nil
	commit.ContactAutomation.Status = domain.ContactAutomationStatusCompleted
	commit.ContactAutomation.ScheduledAt = nil
	committed, err := automationRepo.CommitContactAutomationState(
		context.Background(), workspace.ID, *winners[0], commit,
	)
	require.NoError(t, err)
	assert.True(t, committed)

	var stateVersion int64
	var status string
	var claimToken any
	err = db.QueryRow(`
		SELECT state_version, status, claim_token
		FROM contact_automations WHERE id = 'ca-lease-test'
	`).Scan(&stateVersion, &status, &claimToken)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stateVersion)
	assert.Equal(t, "completed", status)
	assert.Nil(t, claimToken)
}
