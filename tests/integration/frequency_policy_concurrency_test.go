package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/realtimecache"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrequencyPolicyConcurrencyIntegration(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_REDIS_ADDR is not configured")
	}
	client := redis.NewClient(&redis.Options{Addr: address, Password: os.Getenv("TEST_REDIS_PASSWORD")})
	store, err := realtimecache.NewRedisStore(client)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Ping(context.Background()))

	workspaceID := "frequency-" + uuid.NewString()
	customerID := uuid.NewString()
	windows := []realtimecache.WindowReservation{
		{PolicyID: "campaign", Window: time.Hour, MaxEvents: 17},
		{PolicyID: "trigger", Window: time.Hour, MaxEvents: 11},
		{PolicyID: "workspace-global", Window: time.Hour, MaxEvents: 7},
	}
	for _, window := range windows {
		key, keyErr := realtimecache.FrequencyKey(workspaceID, customerID, "email", window.PolicyID)
		require.NoError(t, keyErr)
		t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })
	}

	const workers = 100
	start := make(chan struct{})
	results := make(chan struct {
		reservation string
		allowed     bool
		err         error
	}, workers)
	var group sync.WaitGroup
	now := time.Now().UTC()
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			<-start
			reservation := fmt.Sprintf("reservation-%d", worker)
			result, reserveErr := store.ReserveWindows(context.Background(), workspaceID, customerID, "email", reservation, now, windows)
			results <- struct {
				reservation string
				allowed     bool
				err         error
			}{reservation: reservation, allowed: result.Allowed, err: reserveErr}
		}(worker)
	}
	close(start)
	group.Wait()
	close(results)
	allowed := 0
	allowedReservation := ""
	for result := range results {
		require.NoError(t, result.err)
		if result.allowed {
			allowed++
			allowedReservation = result.reservation
		}
	}
	assert.Equal(t, 7, allowed, "the strictest Workspace-wide cap must win atomically")
	require.NotEmpty(t, allowedReservation)
	replay, err := store.ReserveWindows(context.Background(), workspaceID, customerID, "email", allowedReservation, now, windows)
	require.NoError(t, err)
	assert.True(t, replay.Allowed, "replaying an accepted effect must be idempotent")
	denied, err := store.ReserveWindows(context.Background(), workspaceID, customerID, "email", "after-limit", now, windows)
	require.NoError(t, err)
	assert.False(t, denied.Allowed)
}
