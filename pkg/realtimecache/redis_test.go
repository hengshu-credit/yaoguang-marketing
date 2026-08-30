package realtimecache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriggerCacheKeyIsTenantAndVersionScoped(t *testing.T) {
	first, err := TriggerKey("workspace-1", "automation-1", 7)
	require.NoError(t, err)
	second, err := TriggerKey("workspace-2", "automation-1", 7)
	require.NoError(t, err)
	third, err := TriggerKey("workspace-1", "automation-1", 8)
	require.NoError(t, err)

	assert.Contains(t, first, "workspace-1")
	assert.Contains(t, first, ":v7")
	assert.NotEqual(t, first, second)
	assert.NotEqual(t, first, third)
}

func TestRedisTriggerCacheRequiresTTLAndExpires(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	defer store.Close()

	entry := TriggerEntry{AutomationVersion: 3, CachedAt: time.Now().UTC(), Payload: json.RawMessage(`{"query":"SELECT true"}`)}
	require.Error(t, store.SetTrigger(context.Background(), "workspace-1", "automation-1", entry, 0))
	require.NoError(t, store.SetTrigger(context.Background(), "workspace-1", "automation-1", entry, 30*time.Second))

	key, err := TriggerKey("workspace-1", "automation-1", 3)
	require.NoError(t, err)
	assert.Greater(t, server.TTL(key), time.Duration(0))
	loaded, found, err := store.GetTrigger(context.Background(), "workspace-1", "automation-1", 3)
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, string(entry.Payload), string(loaded.Payload))

	server.FastForward(31 * time.Second)
	_, found, err = store.GetTrigger(context.Background(), "workspace-1", "automation-1", 3)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestRedisSlidingWindowIsAtomicAndAlwaysExpires(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	const workers = 20
	var allowed atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			result, reserveErr := store.ReserveSlidingWindow(
				context.Background(), "workspace-1", "person@example.com", "email", "daily",
				fmt.Sprintf("reservation-%d", worker), now, time.Hour, 5,
			)
			if reserveErr != nil {
				errors <- reserveErr
				return
			}
			if result.Allowed {
				allowed.Add(1)
			}
		}(worker)
	}
	wg.Wait()
	close(errors)
	for reserveErr := range errors {
		require.NoError(t, reserveErr)
	}
	assert.Equal(t, int32(5), allowed.Load())

	key, err := FrequencyKey("workspace-1", "person@example.com", "email", "daily")
	require.NoError(t, err)
	assert.Greater(t, server.TTL(key), time.Duration(0))
}

func TestRedisMultiPolicyReservationIsAllOrNothingAndReplaySafe(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	defer store.Close()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	windows := []WindowReservation{{PolicyID: "campaign:v1", Window: time.Hour, MaxEvents: 2}, {PolicyID: "global:v3", Window: 24 * time.Hour, MaxEvents: 1}}

	first, err := store.ReserveWindows(context.Background(), "workspace-1", "customer-1", "email", "effect-1", now, windows)
	require.NoError(t, err)
	assert.True(t, first.Allowed)
	replay, err := store.ReserveWindows(context.Background(), "workspace-1", "customer-1", "email", "effect-1", now, windows)
	require.NoError(t, err)
	assert.True(t, replay.Allowed)
	assert.True(t, replay.Replayed)

	denied, err := store.ReserveWindows(context.Background(), "workspace-1", "customer-1", "email", "effect-2", now, windows)
	require.NoError(t, err)
	assert.False(t, denied.Allowed)
	assert.Equal(t, "global:v3", denied.DeniedPolicyID)
	campaignKey, err := FrequencyKey("workspace-1", "customer-1", "email", "campaign:v1")
	require.NoError(t, err)
	count, err := client.ZCard(context.Background(), campaignKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "denied reservation must not consume the campaign allowance")
}
