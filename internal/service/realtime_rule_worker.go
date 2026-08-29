package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/broker"
)

var (
	ErrInvalidRuleMessage = errors.New("invalid rule event message")
	ErrRuleInboxBusy      = errors.New("rule event is already being processed")
)

type RuleWorker struct {
	processor  domain.RuleEventProcessor
	mode       config.RealtimeMode
	inboxLease time.Duration
	now        func() time.Time
}

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
	if envelope.Subject.ContactEmail == "" {
		return fmt.Errorf("%w: contact_email is required for rule matching", ErrInvalidRuleMessage)
	}

	result, err := w.processor.ProcessRuleEvent(ctx, domain.RuleProcessRequest{
		WorkspaceID:    envelope.WorkspaceID,
		Consumer:       "rule-worker",
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
