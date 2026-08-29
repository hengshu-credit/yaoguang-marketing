package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

var ErrInvalidJourneyMessage = errors.New("invalid journey wake message")

// JourneyWorker treats RabbitMQ as a wake-up signal only. PostgreSQL decides
// whether the journey is due, which worker owns it, and whether the transition
// still applies to the same state version.
type JourneyWorker struct {
	repository domain.JourneyClaimRepository
	planner    domain.JourneyStepPlanner
	workerID   string
	lease      time.Duration
	now        func() time.Time
}

func NewJourneyWorker(
	repository domain.JourneyClaimRepository,
	planner domain.JourneyStepPlanner,
	workerID string,
	lease time.Duration,
) (*JourneyWorker, error) {
	if repository == nil {
		return nil, errors.New("journey claim repository is required")
	}
	if planner == nil {
		return nil, errors.New("journey step planner is required")
	}
	if strings.TrimSpace(workerID) == "" {
		return nil, errors.New("journey worker id is required")
	}
	if lease <= 0 {
		return nil, errors.New("journey claim lease must be positive")
	}
	return &JourneyWorker{
		repository: repository,
		planner:    planner,
		workerID:   workerID,
		lease:      lease,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func (w *JourneyWorker) Handle(ctx context.Context, message broker.Message) error {
	if message.ID == uuid.Nil {
		return fmt.Errorf("%w: message id is required", ErrInvalidJourneyMessage)
	}
	var envelope domain.EventEnvelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return fmt.Errorf("%w: decode event envelope: %v", ErrInvalidJourneyMessage, err)
	}
	if envelope.ID != message.ID {
		return fmt.Errorf("%w: envelope id does not match message id", ErrInvalidJourneyMessage)
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJourneyMessage, err)
	}
	if !strings.HasPrefix(envelope.Type, "journey.") {
		return fmt.Errorf("%w: unsupported event type %q", ErrInvalidJourneyMessage, envelope.Type)
	}
	if envelope.Subject.Type != "contact_automation" || strings.TrimSpace(envelope.Subject.ID) == "" {
		return fmt.Errorf("%w: contact_automation subject is required", ErrInvalidJourneyMessage)
	}

	now := w.now().UTC()
	claim, acquired, err := w.repository.ClaimContactAutomation(
		ctx, envelope.WorkspaceID, envelope.Subject.ID, w.workerID, now, w.lease,
	)
	if err != nil {
		return err
	}
	if !acquired {
		// Another worker owns the lease, the workflow is not due, or it already
		// reached a terminal state. The database scheduler will wake it again if
		// necessary, so requeueing this transport message would only create heat.
		return nil
	}

	commit, err := w.planner.PlanClaimedJourneyStep(ctx, envelope.WorkspaceID, *claim, now)
	if err != nil {
		_, releaseErr := w.repository.ReleaseContactAutomationClaim(
			ctx, envelope.WorkspaceID, claim.ContactAutomation.ID, claim.ClaimToken,
		)
		if releaseErr != nil {
			return errors.Join(err, releaseErr)
		}
		return err
	}
	committed, err := w.repository.CommitContactAutomationState(ctx, envelope.WorkspaceID, *claim, commit)
	if err != nil {
		return err
	}
	if !committed {
		// A stale token/state version means another authorized transition won.
		// Acknowledging the stale wake is safe because its desired state is no
		// longer current.
		return nil
	}
	return nil
}

func (w *JourneyWorker) HandleDelivery(ctx context.Context, message broker.Message) broker.DeliveryDecision {
	err := w.Handle(ctx, message)
	switch {
	case err == nil:
		return broker.DeliveryDecision{Action: broker.Ack}
	case errors.Is(err, ErrInvalidJourneyMessage):
		return broker.DeliveryDecision{Action: broker.DeadLetter, Err: err}
	default:
		return broker.DeliveryDecision{Action: broker.Retry, RetryTier: broker.Retry30Seconds, Err: err}
	}
}
