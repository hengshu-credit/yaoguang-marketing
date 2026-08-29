package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/realtimecache"
)

type fakeFrequencyWindowStore struct {
	result realtimecache.WindowResult
	err    error
	calls  int
}

func (s *fakeFrequencyWindowStore) ReserveSlidingWindow(
	context.Context, string, string, string, string, string, time.Time, time.Duration, int,
) (realtimecache.WindowResult, error) {
	s.calls++
	return s.result, s.err
}

func TestFrequencyLimiterAllowsWhenPolicyIsDisabled(t *testing.T) {
	store := &fakeFrequencyWindowStore{err: errors.New("redis unavailable")}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)

	decision, err := limiter.Allow(context.Background(), "workspace-1", "person@example.com", "email", FrequencyPolicy{}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, FrequencyDecisionAllow, decision)
	assert.Zero(t, store.calls)
}

func TestFrequencyLimiterFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	store := &fakeFrequencyWindowStore{err: errors.New("redis unavailable")}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)

	decision, err := limiter.Allow(context.Background(), "workspace-1", "person@example.com", "email", FrequencyPolicy{
		ID: "daily", MaxEvents: 3, Window: 24 * time.Hour, ReservationID: "effect-1",
	}, time.Now())
	require.Error(t, err)
	assert.Equal(t, FrequencyDecisionDefer, decision)
}

func TestFrequencyLimiterReturnsDenyWhenWindowIsFull(t *testing.T) {
	store := &fakeFrequencyWindowStore{result: realtimecache.WindowResult{Allowed: false, Count: 3, RetryAfter: time.Hour}}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)

	decision, err := limiter.Allow(context.Background(), "workspace-1", "person@example.com", "email", FrequencyPolicy{
		ID: "daily", MaxEvents: 3, Window: 24 * time.Hour, ReservationID: "effect-1",
	}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, FrequencyDecisionDeny, decision)
}

func TestFrequencyLimiterRejectsUnscopedRequests(t *testing.T) {
	limiter, err := NewFrequencyLimiter(&fakeFrequencyWindowStore{})
	require.NoError(t, err)
	decision, err := limiter.Allow(context.Background(), "", "person@example.com", "email", FrequencyPolicy{
		ID: "daily", MaxEvents: 1, Window: time.Hour,
	}, time.Now())
	require.Error(t, err)
	assert.Equal(t, FrequencyDecisionDefer, decision)
}
