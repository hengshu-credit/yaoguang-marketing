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

type fakeMultiFrequencyStore struct {
	fakeFrequencyWindowStore
	multiResult realtimecache.MultiWindowResult
	multiErr    error
	windows     []realtimecache.WindowReservation
}

func (s *fakeMultiFrequencyStore) ReserveWindows(_ context.Context, _, _, _, _ string, _ time.Time, windows []realtimecache.WindowReservation) (realtimecache.MultiWindowResult, error) {
	s.windows = windows
	return s.multiResult, s.multiErr
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

func TestFrequencyLimiterReservesCampaignTriggerAndGlobalPoliciesAtomically(t *testing.T) {
	store := &fakeMultiFrequencyStore{multiResult: realtimecache.MultiWindowResult{Allowed: true}}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)
	result, err := limiter.AllowAll(context.Background(), "workspace-1", "customer-1", "email", "effect-1", []FrequencyPolicy{
		{ID: "campaign", Version: 1, Scope: "campaign", MaxEvents: 1, Window: time.Hour},
		{ID: "trigger", Version: 2, Scope: "trigger", MaxEvents: 2, Window: 24 * time.Hour},
		{ID: "global", Version: 3, Scope: "workspace_global", MaxEvents: 3, Window: 7 * 24 * time.Hour},
	}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, FrequencyDecisionAllow, result.Decision)
	require.Len(t, store.windows, 3)
	assert.Equal(t, "campaign:v1", store.windows[0].PolicyID)
	assert.Equal(t, "global:v3", store.windows[2].PolicyID)
}

func TestFrequencyLimiterMultiPolicyFailsClosedWithoutAtomicStore(t *testing.T) {
	limiter, err := NewFrequencyLimiter(&fakeFrequencyWindowStore{})
	require.NoError(t, err)
	result, err := limiter.AllowAll(context.Background(), "workspace-1", "customer-1", "email", "effect-1", []FrequencyPolicy{{ID: "global", MaxEvents: 1, Window: time.Hour}}, time.Now())
	require.Error(t, err)
	assert.Equal(t, FrequencyDecisionDefer, result.Decision)
}

func TestFrequencyLimiterMultiPolicyDenyIdentifiesLayerWithoutPartialReservation(t *testing.T) {
	store := &fakeMultiFrequencyStore{multiResult: realtimecache.MultiWindowResult{Allowed: false, DeniedPolicyID: "trigger:v2", RetryAfter: time.Hour}}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)
	result, err := limiter.AllowAll(context.Background(), "workspace-1", "customer-1", "email", "effect-1", []FrequencyPolicy{{ID: "trigger", Version: 2, MaxEvents: 1, Window: time.Hour}}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, FrequencyDecisionDeny, result.Decision)
	assert.Equal(t, "trigger:v2", result.DeniedPolicyID)
}
