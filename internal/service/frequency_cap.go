package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/pkg/realtimecache"
)

type FrequencyDecision string

const (
	FrequencyDecisionAllow FrequencyDecision = "allow"
	FrequencyDecisionDeny  FrequencyDecision = "deny"
	FrequencyDecisionDefer FrequencyDecision = "defer"
)

type FrequencyPolicy struct {
	ID            string
	MaxEvents     int
	Window        time.Duration
	ReservationID string
}

func (p FrequencyPolicy) enabled() bool {
	return p.ID != "" || p.MaxEvents != 0 || p.Window != 0 || p.ReservationID != ""
}

type FrequencyLimiter struct {
	store realtimecache.FrequencyWindowStore
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
