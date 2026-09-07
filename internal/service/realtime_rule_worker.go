package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

var (
	ErrInvalidRuleMessage   = errors.New("invalid rule event message")
	ErrRuleInboxBusy        = errors.New("rule event is already being processed")
	ErrJourneyEntryDeferred = errors.New("journey entry guard deferred the event")
)

type RuleWorker struct {
	processor      domain.RuleEventProcessor
	projector      EventStateProjector
	configuredOnly bool
	mode           config.RealtimeMode
	inboxLease     time.Duration
	now            func() time.Time
}

type EventStateProjector interface {
	ProcessEvent(context.Context, string, domain.EventEnvelope) error
}

func (w *RuleWorker) SetStateProjector(projector EventStateProjector) { w.projector = projector }
func (w *RuleWorker) SetConfiguredOnly(only bool)                     { w.configuredOnly = only }

func NewRuleWorker(
	processor domain.RuleEventProcessor,
	mode config.RealtimeMode,
	inboxLease time.Duration,
) (*RuleWorker, error) {
	if processor == nil {
		return nil, errors.New("rule event processor is required")
	}
	if mode != config.RealtimeModeShadow && mode != config.RealtimeModePrimary {
		return nil, fmt.Errorf("rule worker requires shadow or primary mode, got %q", mode)
	}
	if inboxLease <= 0 {
		return nil, errors.New("rule worker inbox lease must be positive")
	}
	return &RuleWorker{
		processor:  processor,
		mode:       mode,
		inboxLease: inboxLease,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func (w *RuleWorker) Handle(ctx context.Context, message broker.Message) error {
	if message.ID == uuid.Nil {
		return fmt.Errorf("%w: message id is required", ErrInvalidRuleMessage)
	}
	var envelope domain.EventEnvelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return fmt.Errorf("%w: decode event envelope: %v", ErrInvalidRuleMessage, err)
	}
	if envelope.ID != message.ID {
		return fmt.Errorf("%w: envelope id does not match message id", ErrInvalidRuleMessage)
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRuleMessage, err)
	}
	var metadata struct {
		ProcessingStatus string     `json:"processing_status"`
		ExpiresAt        *time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(envelope.Data, &metadata); err != nil {
		return fmt.Errorf("%w: invalid event metadata: %v", ErrInvalidRuleMessage, err)
	}
	if metadata.ProcessingStatus == "pending_identity" {
		return nil
	}
	if envelope.Subject.ContactEmail == "" && envelope.Subject.CustomerID == "" && envelope.Subject.Type != "customer" {
		if metadata.ProcessingStatus == "expired" || metadata.ProcessingStatus == "backfill" || metadata.ProcessingStatus == "associated_history" {
			return nil
		}
		return fmt.Errorf("%w: customer identity is required for rule matching", ErrInvalidRuleMessage)
	}
	if w.projector != nil && (envelope.Subject.CustomerID != "" || envelope.Subject.Type == "customer") {
		if err := w.projector.ProcessEvent(ctx, envelope.WorkspaceID, envelope); err != nil {
			return fmt.Errorf("project event state: %w", err)
		}
	}
	// The ledger and analytical projections retain historical reports. Eligibility
	// is checked again at dispatch because a queued report may expire while waiting.
	if metadata.ProcessingStatus != "" && metadata.ProcessingStatus != "accepted" {
		return nil
	}
	if metadata.ExpiresAt != nil && !w.now().Before(*metadata.ExpiresAt) {
		return nil
	}

	consumer := "rule-worker"
	if w.configuredOnly {
		consumer = "configured-rule-worker"
	}
	result, err := w.processor.ProcessRuleEvent(ctx, domain.RuleProcessRequest{
		ConfiguredOnly: w.configuredOnly,
		WorkspaceID:    envelope.WorkspaceID,
		Consumer:       consumer,
		MessageID:      message.ID,
		Envelope:       envelope,
		DependencyKeys: eventDependencyKeys(envelope.Data),
		Engine:         domain.MatchEngineRealtime,
		Primary:        w.mode == config.RealtimeModePrimary,
		Now:            w.now().UTC(),
		InboxLease:     w.inboxLease,
	})
	if err != nil {
		return err
	}
	if result.Busy {
		return ErrRuleInboxBusy
	}
	if result.Deferred > 0 {
		return ErrJourneyEntryDeferred
	}
	return nil
}

func (w *RuleWorker) HandleDelivery(ctx context.Context, message broker.Message) broker.DeliveryDecision {
	err := w.Handle(ctx, message)
	switch {
	case err == nil:
		return broker.DeliveryDecision{Action: broker.Ack}
	case errors.Is(err, ErrInvalidRuleMessage):
		return broker.DeliveryDecision{Action: broker.DeadLetter, Err: err}
	case errors.Is(err, ErrRuleInboxBusy):
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry5Seconds, Err: err}
	case errors.Is(err, ErrJourneyEntryDeferred):
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry5Minutes, Err: err}
	default:
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: err}
	}
}

func eventDependencyKeys(data json.RawMessage) []string {
	var indexed struct {
		EntityID string                     `json:"entity_id"`
		Changes  map[string]json.RawMessage `json:"changes"`
	}
	if json.Unmarshal(data, &indexed) != nil {
		return nil
	}
	keys := make([]string, 0, len(indexed.Changes)+1)
	if indexed.EntityID != "" {
		keys = append(keys, "entity_id:"+indexed.EntityID)
	}
	for field := range indexed.Changes {
		keys = append(keys, "changes."+field)
	}
	sort.Strings(keys)
	return keys
}
