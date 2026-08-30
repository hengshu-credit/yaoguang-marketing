package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/realtimecache"
)

type FrequencyDecision string

const (
	FrequencyDecisionAllow FrequencyDecision = "allow"
	FrequencyDecisionDeny  FrequencyDecision = "deny"
	FrequencyDecisionDefer FrequencyDecision = "defer"
)

type FrequencyPolicy struct {
	ID            string
	Version       int
	Scope         string
	MaxEvents     int
	Window        time.Duration
	ReservationID string
}

type MultiFrequencyResult struct {
	Decision       FrequencyDecision
	DeniedPolicyID string
	RetryAfter     time.Duration
	Replayed       bool
}

func (p FrequencyPolicy) enabled() bool {
	return p.ID != "" || p.MaxEvents != 0 || p.Window != 0 || p.ReservationID != ""
}

type FrequencyLimiter struct {
	store realtimecache.FrequencyWindowStore
}

type unavailableFrequencyStore struct{ reason error }

func (s unavailableFrequencyStore) ReserveSlidingWindow(context.Context, string, string, string, string, string, time.Time, time.Duration, int) (realtimecache.WindowResult, error) {
	return realtimecache.WindowResult{}, s.reason
}
func (s unavailableFrequencyStore) ReserveWindows(context.Context, string, string, string, string, time.Time, []realtimecache.WindowReservation) (realtimecache.MultiWindowResult, error) {
	return realtimecache.MultiWindowResult{}, s.reason
}

func NewUnavailableFrequencyLimiter(reason error) *FrequencyLimiter {
	if reason == nil {
		reason = errors.New("frequency store is not configured")
	}
	return &FrequencyLimiter{store: unavailableFrequencyStore{reason: reason}}
}

func NewFrequencyLimiter(store realtimecache.FrequencyWindowStore) (*FrequencyLimiter, error) {
	if store == nil {
		return nil, errors.New("frequency window store is required")
	}
	return &FrequencyLimiter{store: store}, nil
}

// Allow fails closed for configured frequency policies. Callers should retry a
// Defer decision after infrastructure recovery, while Deny is a business result
// that may be retried only after the returned policy window advances.
func (l *FrequencyLimiter) Allow(
	ctx context.Context,
	workspaceID, subjectID, channel string,
	policy FrequencyPolicy,
	now time.Time,
) (FrequencyDecision, error) {
	if !policy.enabled() {
		return FrequencyDecisionAllow, nil
	}
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(subjectID) == "" || strings.TrimSpace(channel) == "" {
		return FrequencyDecisionDefer, errors.New("workspace, subject, and channel are required for frequency control")
	}
	if strings.TrimSpace(policy.ID) == "" || policy.MaxEvents <= 0 || policy.Window <= 0 || strings.TrimSpace(policy.ReservationID) == "" {
		return FrequencyDecisionDefer, errors.New("frequency policy id, max events, window, and reservation id are required")
	}
	result, err := l.store.ReserveSlidingWindow(
		ctx, workspaceID, subjectID, channel, policy.ID, policy.ReservationID,
		now.UTC(), policy.Window, policy.MaxEvents,
	)
	if err != nil {
		return FrequencyDecisionDefer, fmt.Errorf("frequency control unavailable: %w", err)
	}
	if !result.Allowed {
		return FrequencyDecisionDeny, nil
	}
	return FrequencyDecisionAllow, nil
}

// AllowAll evaluates campaign, trigger and workspace-global policies in one
// atomic reservation. It never falls back to sequential reservations because
// that could consume only part of the applicable allowance.
func (l *FrequencyLimiter) AllowAll(
	ctx context.Context,
	workspaceID, customerID, channel, reservationID string,
	policies []FrequencyPolicy,
	now time.Time,
) (MultiFrequencyResult, error) {
	if len(policies) == 0 {
		return MultiFrequencyResult{Decision: FrequencyDecisionAllow}, nil
	}
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(customerID) == "" || strings.TrimSpace(channel) == "" || strings.TrimSpace(reservationID) == "" {
		return MultiFrequencyResult{Decision: FrequencyDecisionDefer}, errors.New("workspace, customer, channel, and reservation are required for frequency control")
	}
	windows := make([]realtimecache.WindowReservation, 0, len(policies))
	seen := map[string]struct{}{}
	for _, policy := range policies {
		if strings.TrimSpace(policy.ID) == "" || policy.MaxEvents <= 0 || policy.Window <= 0 {
			return MultiFrequencyResult{Decision: FrequencyDecisionDefer}, errors.New("every frequency policy requires id, max events, and window")
		}
		policyKey := policy.ID
		if policy.Version > 0 {
			policyKey = fmt.Sprintf("%s:v%d", policy.ID, policy.Version)
		}
		if _, duplicate := seen[policyKey]; duplicate {
			return MultiFrequencyResult{Decision: FrequencyDecisionDefer}, errors.New("frequency policy versions must be unique")
		}
		seen[policyKey] = struct{}{}
		windows = append(windows, realtimecache.WindowReservation{PolicyID: policyKey, Window: policy.Window, MaxEvents: policy.MaxEvents})
	}
	multiStore, ok := l.store.(realtimecache.MultiFrequencyWindowStore)
	if !ok {
		return MultiFrequencyResult{Decision: FrequencyDecisionDefer}, errors.New("atomic multi-policy frequency control is unavailable")
	}
	result, err := multiStore.ReserveWindows(ctx, workspaceID, customerID, channel, reservationID, now.UTC(), windows)
	if err != nil {
		return MultiFrequencyResult{Decision: FrequencyDecisionDefer}, fmt.Errorf("frequency control unavailable: %w", err)
	}
	if !result.Allowed {
		return MultiFrequencyResult{Decision: FrequencyDecisionDeny, DeniedPolicyID: result.DeniedPolicyID, RetryAfter: result.RetryAfter}, nil
	}
	return MultiFrequencyResult{Decision: FrequencyDecisionAllow, Replayed: result.Replayed}, nil
}
