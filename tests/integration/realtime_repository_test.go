package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/tests/testutil"
)

func TestRealtimeRepositoryConcurrentClaim(t *testing.T) {
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

	messageID := uuid.New()
	eventID := uuid.New()
	_, err = db.Exec(`
		INSERT INTO event_outbox (id, event_id, topic, routing_key, payload)
		VALUES ($1, $2, 'notifuse.events', 'contact.updated', $3::jsonb)
	`, messageID, eventID, fmt.Sprintf(`{"id":%q,"event_id":%q}`, messageID, eventID))
	require.NoError(t, err)

	realtime := repository.NewRealtimeRepositoryWithDB(db)
	start := make(chan struct{})
	results := make(chan int, 10)
	errors := make(chan error, 10)
	var workers sync.WaitGroup
	for worker := range 10 {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			<-start
			claimed, claimErr := realtime.ClaimOutbox(
				context.Background(), workspace.ID, fmt.Sprintf("worker-%d", worker),
				time.Now().UTC(), time.Minute, 1,
			)
			if claimErr != nil {
				errors <- claimErr
				return
			}
			results <- len(claimed)
		}(worker)
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)

	for claimErr := range errors {
		require.NoError(t, claimErr)
	}
	total := 0
	for count := range results {
		total += count
	}
	assert.Equal(t, 1, total, "exactly one worker must own the outbox row")
}
