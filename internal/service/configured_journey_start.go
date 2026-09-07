package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type configuredJourneyStartGate interface {
	ConfiguredJourneyStart(context.Context, string, string, time.Time) (string, *time.Time, error)
}

func (e *AutomationExecutor) checkConfiguredStart(ctx context.Context, w string, journey *domain.ContactAutomation, now time.Time) (string, *time.Time, error) {
	if e.journeyStartGate == nil {
		return "", nil, nil
	}
	reason, expiry, err := e.journeyStartGate.ConfiguredJourneyStart(ctx, w, journey.ID, now)
	if err != nil || reason != "" {
		return reason, expiry, err
	}
	copy := map[string]any{}
	for key, value := range journey.Context {
		copy[key] = value
	}
	copy["business_started_at"] = now.UTC().Format(time.RFC3339Nano)
	if expiry != nil {
		copy["business_start_expires_at"] = expiry.UTC().Format(time.RFC3339Nano)
	}
	journey.Context = copy
	return "", expiry, nil
}

func (e *AutomationExecutor) recordExpiredInitialEffect(ctx context.Context, w string, command journeySideEffectData, effectKey string) error {
	if e.executionSnapshots == nil || domain.ExecutionIDFromContext(ctx) == "" {
		return nil
	}
	return e.executionSnapshots.AppendInternal(ctx, w, domain.ExecutionEvidence{ExecutionID: domain.ExecutionIDFromContext(ctx), Key: effectKey + ":origin_event_expired", Kind: "node", NodeID: command.NodeID, CustomerID: command.CustomerID, Status: "skipped", Payload: []byte(`{"reason":"origin_event_expired","side_effect_executed":false}`)})
}
