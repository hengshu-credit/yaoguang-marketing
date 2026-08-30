package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryConcurrencyIntegration(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	for ordinal := 1; ordinal <= 40; ordinal++ {
		fixture.reserve(t, ordinal)
	}
	var wait sync.WaitGroup
	claimedIDs := make(chan string, 40)
	for worker := 0; worker < 4; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			claims, err := fixture.queue.ClaimPending(context.Background(), fixture.workspaceID, 10, time.Minute)
			assert.NoError(t, err)
			for _, claim := range claims {
				claimedIDs <- claim.ID
			}
		}()
	}
	wait.Wait()
	close(claimedIDs)
	unique := map[string]struct{}{}
	for id := range claimedIDs {
		_, duplicate := unique[id]
		assert.False(t, duplicate, "one queue row was claimed by multiple workers")
		unique[id] = struct{}{}
	}
	require.Len(t, unique, 40)
	progress, err := fixture.repository.GetDeliveryProgress(context.Background(), fixture.workspaceID, domain.DeliverySourceBroadcast, "broadcast-crash", "1")
	require.NoError(t, err)
	assert.Equal(t, int64(40), progress.AudienceTotal)
}
