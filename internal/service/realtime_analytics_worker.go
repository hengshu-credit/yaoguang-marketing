package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/eventanalytics"
)

var ErrInvalidAnalyticsMessage = errors.New("invalid analytics event message")

type AnalyticsWorker struct {
	store      eventanalytics.EventProjectionStore
	inbox      domain.ConsumerInboxRepository
	inboxLease time.Duration
	now        func() time.Time
}

func NewAnalyticsWorker(
	store eventanalytics.EventProjectionStore,
	inbox domain.ConsumerInboxRepository,
	inboxLease time.Duration,
) (*AnalyticsWorker, error) {
	if store == nil {
		return nil, errors.New("event projection store is required")
	}
	if inbox == nil {
		return nil, errors.New("analytics inbox repository is required")
	}
	if inboxLease <= 0 {
		return nil, errors.New("analytics inbox lease must be positive")
	}
	return &AnalyticsWorker{
		store: store, inbox: inbox, inboxLease: inboxLease,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

type claimedAnalyticsEvent struct {
	inputIndex int
	envelope   domain.EventEnvelope
	claim      domain.InboxClaim
}

func (w *AnalyticsWorker) HandleBatch(ctx context.Context, messages []broker.Message) []broker.DeliveryDecision {
	decisions := make([]broker.DeliveryDecision, len(messages))
	claimed := make([]claimedAnalyticsEvent, 0, len(messages))
	now := w.now().UTC()
	for index, message := range messages {
		envelope, err := decodeAnalyticsMessage(message)
		if err != nil {
			decisions[index] = broker.DeliveryDecision{Action: broker.DeadLetter, Err: err}
			continue
		}
		claim, err := w.inbox.ClaimConsumerMessage(
			ctx, envelope.WorkspaceID, "analytics-worker", message.ID, now, w.inboxLease,
		)
		if err != nil {
			decisions[index] = broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: err}
			continue
		}
		if !claim.Acquired {
			if claim.Status == domain.InboxStatusCompleted {
				decisions[index] = broker.DeliveryDecision{Action: broker.Ack}
			} else {
				decisions[index] = broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry5Seconds}
			}
			continue
		}
		claimed = append(claimed, claimedAnalyticsEvent{inputIndex: index, envelope: envelope, claim: claim})
	}
	if len(claimed) == 0 {
		return decisions
	}

	// Group tenants while preserving chronological event order within each
	// tenant. This makes ClickHouse parts more compressible without violating
	// per-tenant event ordering.
	sort.SliceStable(claimed, func(left, right int) bool {
		if claimed[left].envelope.WorkspaceID != claimed[right].envelope.WorkspaceID {
			return claimed[left].envelope.WorkspaceID < claimed[right].envelope.WorkspaceID
		}
		if !claimed[left].envelope.OccurredAt.Equal(claimed[right].envelope.OccurredAt) {
			return claimed[left].envelope.OccurredAt.Before(claimed[right].envelope.OccurredAt)
		}
		return claimed[left].envelope.EventID.String() < claimed[right].envelope.EventID.String()
	})
	events := make([]domain.EventEnvelope, len(claimed))
	for index := range claimed {
		events[index] = claimed[index].envelope
	}
	if err := w.store.InsertBatch(ctx, events); err != nil {
		for _, item := range claimed {
			_, failErr := w.inbox.FailConsumerMessage(
				ctx, item.envelope.WorkspaceID, "analytics-worker",
				item.claim.MessageID, item.claim.ClaimToken, now, err.Error(),
			)
			decisionErr := err
			if failErr != nil {
				decisionErr = errors.Join(err, failErr)
			}
			decisions[item.inputIndex] = broker.DeliveryDecision{
				Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: decisionErr,
			}
		}
		return decisions
	}

	for _, item := range claimed {
		completed, err := w.inbox.CompleteConsumerMessage(
			ctx, item.envelope.WorkspaceID, "analytics-worker",
			item.claim.MessageID, item.claim.ClaimToken, now,
		)
		if err != nil || !completed {
			if err == nil {
				err = errors.New("analytics inbox claim lost before completion")
			}
			decisions[item.inputIndex] = broker.DeliveryDecision{
				Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: err,
			}
			continue
		}
		decisions[item.inputIndex] = broker.DeliveryDecision{Action: broker.Ack}
	}
	return decisions
}

func decodeAnalyticsMessage(message broker.Message) (domain.EventEnvelope, error) {
	if message.ID == uuid.Nil {
		return domain.EventEnvelope{}, fmt.Errorf("%w: message id is required", ErrInvalidAnalyticsMessage)
	}
	var envelope domain.EventEnvelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return domain.EventEnvelope{}, fmt.Errorf("%w: decode event envelope: %v", ErrInvalidAnalyticsMessage, err)
	}
	if envelope.ID != message.ID {
		return domain.EventEnvelope{}, fmt.Errorf("%w: envelope id does not match message id", ErrInvalidAnalyticsMessage)
	}
	if err := envelope.Validate(); err != nil {
		return domain.EventEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidAnalyticsMessage, err)
	}
	return envelope, nil
}

type analyticsBatchRequest struct {
	message broker.Message
	result  chan broker.DeliveryDecision
}

// AnalyticsBatcher lets concurrent RabbitMQ handlers coalesce inserts by size
// or interval while each handler still waits for its own ACK/retry decision.
type AnalyticsBatcher struct {
	worker        *AnalyticsWorker
	batchSize     int
	flushInterval time.Duration
	requests      chan analyticsBatchRequest
}

func NewAnalyticsBatcher(worker *AnalyticsWorker, batchSize int, flushInterval time.Duration) (*AnalyticsBatcher, error) {
	if worker == nil {
		return nil, errors.New("analytics worker is required")
	}
	if batchSize <= 0 || flushInterval <= 0 {
		return nil, errors.New("analytics batch size and flush interval must be positive")
	}
	return &AnalyticsBatcher{
		worker: worker, batchSize: batchSize, flushInterval: flushInterval,
		requests: make(chan analyticsBatchRequest, batchSize),
	}, nil
}

func (b *AnalyticsBatcher) HandleDelivery(ctx context.Context, message broker.Message) broker.DeliveryDecision {
	request := analyticsBatchRequest{message: message, result: make(chan broker.DeliveryDecision, 1)}
	select {
	case b.requests <- request:
	case <-ctx.Done():
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: ctx.Err()}
	}
	select {
	case result := <-request.result:
		return result
	case <-ctx.Done():
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: ctx.Err()}
	}
}

func (b *AnalyticsBatcher) Run(ctx context.Context) {
	timer := time.NewTimer(b.flushInterval)
	if !timer.Stop() {
		<-timer.C
	}
	pending := make([]analyticsBatchRequest, 0, b.batchSize)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		messages := make([]broker.Message, len(pending))
		for index := range pending {
			messages[index] = pending[index].message
		}
		decisions := b.worker.HandleBatch(ctx, messages)
		for index := range pending {
			pending[index].result <- decisions[index]
		}
		pending = pending[:0]
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case request := <-b.requests:
			pending = append(pending, request)
			if len(pending) == 1 {
				timer.Reset(b.flushInterval)
			}
			if len(pending) >= b.batchSize {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				flush()
			}
		case <-timer.C:
			flush()
		}
	}
}
