package realtimecache

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisSlidingWindowLiveConcurrency(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_REDIS_ADDR is not configured")
	}
	client := redis.NewClient(&redis.Options{
		Addr: address, Password: os.Getenv("TEST_REDIS_PASSWORD"),
	})
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Ping(context.Background()))

	workspaceID := "integration-" + uuid.NewString()
	key, err := FrequencyKey(workspaceID, "person@example.com", "email", "minute")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	const workers = 50
	const maximum = 7
	start := make(chan struct{})
	results := make(chan bool, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	now := time.Now().UTC()
	for worker := range workers {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			<-start
			result, reserveErr := store.ReserveSlidingWindow(
				context.Background(), workspaceID, "person@example.com", "email", "minute",
				fmt.Sprintf("reservation-%d", worker), now, time.Minute, maximum,
			)
			if reserveErr != nil {
				errors <- reserveErr
				return
			}
			results <- result.Allowed
		}(worker)
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for reserveErr := range errors {
		require.NoError(t, reserveErr)
	}
	allowed := 0
	for result := range results {
		if result {
			allowed++
		}
	}
	assert.Equal(t, maximum, allowed)
	assert.Greater(t, client.PTTL(context.Background(), key).Val(), time.Duration(0))
}
