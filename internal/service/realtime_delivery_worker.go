package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/broker"
)

var (
	ErrInvalidDeliveryMessage   = errors.New("invalid realtime delivery message")
	ErrDeliveryInboxBusy        = errors.New("realtime delivery message is already being processed")
	ErrSideEffectOutcomeUnknown = errors.New("side effect outcome is unknown and requires reconciliation")
)

// JourneySideEffectExecutor materializes one durable journey command. The
// effect key is stable across transport retries and must be propagated to any
// downstream system that supports idempotency keys.
type JourneySideEffectExecutor interface {
	ExecuteJourneySideEffect(context.Context, domain.EventEnvelope, string) error
}

// RealtimeDeliveryWorker uses the PostgreSQL consumer inbox as its duplicate
// boundary. RabbitMQ ACK happens only after the side effect and inbox completion
// both succeed.
type RealtimeDeliveryWorker struct {
	inbox      domain.ConsumerInboxRepository
	effects    domain.SideEffectRepository
	executor   JourneySideEffectExecutor
	inboxLease time.Duration
	now        func() time.Time
}

func NewRealtimeDeliveryWorker(
	inbox domain.ConsumerInboxRepository,
	effects domain.SideEffectRepository,
	executor JourneySideEffectExecutor,
	inboxLease time.Duration,
) (*RealtimeDeliveryWorker, error) {
	if inbox == nil || effects == nil || executor == nil {
		return nil, errors.New("delivery inbox, side effect repository and executor are required")
	}
	if inboxLease <= 0 {
		return nil, errors.New("delivery inbox lease must be positive")
	}
	return &RealtimeDeliveryWorker{
		inbox: inbox, effects: effects, executor: executor, inboxLease: inboxLease,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (w *RealtimeDeliveryWorker) Handle(ctx context.Context, message broker.Message) error {
	envelope, effectKey, err := decodeDeliveryCommand(message)
	if err != nil {
		return err
	}
	now := w.now().UTC()
	claim, err := w.inbox.ClaimConsumerMessage(
		ctx, envelope.WorkspaceID, "delivery-worker", message.ID, now, w.inboxLease,
	)
	if err != nil {
		return err
	}
	if !claim.Acquired {
		if claim.Status == domain.InboxStatusCompleted {
			return nil
		}
		return ErrDeliveryInboxBusy
	}
	execution, err := w.effects.GetSideEffect(ctx, envelope.WorkspaceID, effectKey)
	if err != nil {
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, err)
	}
	if execution.EffectKey != effectKey {
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, fmt.Errorf("%w: side effect key does not match", ErrInvalidDeliveryMessage))
	}
	if execution.RequestHash != "" {
		hash, hashErr := domain.CanonicalJSONHash(message.Body)
		if hashErr != nil {
			return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, hashErr)
		}
		if hash != execution.RequestHash {
			return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, domain.ErrSideEffectHashConflict)
		}
	}
	switch execution.Status {
	case domain.SideEffectStatusConfirmed:
		return w.completeInbox(ctx, envelope.WorkspaceID, message.ID, claim)
	case domain.SideEffectStatusUnknown:
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, ErrSideEffectOutcomeUnknown)
	case domain.SideEffectStatusSubmitted:
		if execution.Channel == "webhook" {
			transitioned, transitionErr := w.effects.TransitionSideEffect(
				ctx, envelope.WorkspaceID, effectKey, domain.SideEffectStatusSubmitted,
				domain.SideEffectStatusUnknown, now, stringPointer(ErrSideEffectOutcomeUnknown.Error()),
			)
			if transitionErr != nil {
				return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, transitionErr)
			}
			if !transitioned {
				return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, errors.New("side effect state changed during reconciliation"))
			}
			return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, ErrSideEffectOutcomeUnknown)
		}
	case domain.SideEffectStatusReserved, domain.SideEffectStatusFailed:
		transitioned, transitionErr := w.effects.TransitionSideEffect(
			ctx, envelope.WorkspaceID, effectKey, execution.Status,
			domain.SideEffectStatusSubmitted, now, nil,
		)
		if transitionErr != nil {
			return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, transitionErr)
		}
		if !transitioned {
			return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, errors.New("side effect state changed before submission"))
		}
	default:
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, fmt.Errorf("invalid side effect status %q", execution.Status))
	}
	if err := w.executor.ExecuteJourneySideEffect(ctx, envelope, effectKey); err != nil {
		outcome := domain.SideEffectStatusFailed
		if errors.Is(err, ErrSideEffectOutcomeUnknown) {
			outcome = domain.SideEffectStatusUnknown
		}
		_, transitionErr := w.effects.TransitionSideEffect(
			ctx, envelope.WorkspaceID, effectKey, domain.SideEffectStatusSubmitted,
			outcome, w.now().UTC(), stringPointer(err.Error()),
		)
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, errors.Join(err, transitionErr))
	}
	transitioned, err := w.effects.TransitionSideEffect(
		ctx, envelope.WorkspaceID, effectKey, domain.SideEffectStatusSubmitted,
		domain.SideEffectStatusConfirmed, w.now().UTC(), nil,
	)
	if err != nil {
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, err)
	}
	if !transitioned {
		return w.failInbox(ctx, envelope.WorkspaceID, message.ID, claim, now, errors.New("side effect state changed before confirmation"))
	}
	return w.completeInbox(ctx, envelope.WorkspaceID, message.ID, claim)
}

func (w *RealtimeDeliveryWorker) completeInbox(
	ctx context.Context, workspaceID string, messageID uuid.UUID, claim domain.InboxClaim,
) error {
	completed, err := w.inbox.CompleteConsumerMessage(
		ctx, workspaceID, "delivery-worker", messageID,
		claim.ClaimToken, w.now().UTC(),
	)
	if err != nil {
		return err
	}
	if !completed {
		return errors.New("delivery inbox claim lost before completion")
	}
	return nil
}

func (w *RealtimeDeliveryWorker) failInbox(
	ctx context.Context,
	workspaceID string,
	messageID uuid.UUID,
	claim domain.InboxClaim,
	failedAt time.Time,
	cause error,
) error {
	_, failErr := w.inbox.FailConsumerMessage(
		ctx, workspaceID, "delivery-worker", messageID, claim.ClaimToken, failedAt, cause.Error(),
	)
	return errors.Join(cause, failErr)
}

func (w *RealtimeDeliveryWorker) HandleDelivery(ctx context.Context, message broker.Message) broker.DeliveryDecision {
	err := w.Handle(ctx, message)
	switch {
	case err == nil:
		return broker.DeliveryDecision{Action: broker.Ack}
	case errors.Is(err, ErrInvalidDeliveryMessage):
		return broker.DeliveryDecision{Action: broker.DeadLetter, Err: err}
	case errors.Is(err, ErrSideEffectOutcomeUnknown), errors.Is(err, domain.ErrSideEffectHashConflict):
		return broker.DeliveryDecision{Action: broker.DeadLetter, Err: err}
	case errors.Is(err, ErrDeliveryInboxBusy):
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry5Seconds, Err: err}
	default:
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: err}
	}
}

func stringPointer(value string) *string { return &value }

func decodeDeliveryCommand(message broker.Message) (domain.EventEnvelope, string, error) {
	if message.ID == uuid.Nil {
		return domain.EventEnvelope{}, "", fmt.Errorf("%w: message id is required", ErrInvalidDeliveryMessage)
	}
	var envelope domain.EventEnvelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return domain.EventEnvelope{}, "", fmt.Errorf("%w: decode envelope: %v", ErrInvalidDeliveryMessage, err)
	}
	if envelope.ID != message.ID {
		return domain.EventEnvelope{}, "", fmt.Errorf("%w: envelope id does not match message id", ErrInvalidDeliveryMessage)
	}
	if err := envelope.Validate(); err != nil {
		return domain.EventEnvelope{}, "", fmt.Errorf("%w: %v", ErrInvalidDeliveryMessage, err)
	}
	if envelope.Type != "journey.side_effect.requested" {
		return domain.EventEnvelope{}, "", fmt.Errorf("%w: unsupported event type %q", ErrInvalidDeliveryMessage, envelope.Type)
	}
	effectKey, ok := message.Headers["effect_key"].(string)
	if !ok || strings.TrimSpace(effectKey) == "" {
		return domain.EventEnvelope{}, "", fmt.Errorf("%w: effect_key header is required", ErrInvalidDeliveryMessage)
	}
	return envelope, effectKey, nil
}
