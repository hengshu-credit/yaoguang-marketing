package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/broker"
)

type recordingDeliveryInbox struct {
	claim        domain.InboxClaim
	claimErr     error
	completed    bool
	completeCall int
	failCall     int
}

func (i *recordingDeliveryInbox) ClaimConsumerMessage(context.Context, string, string, uuid.UUID, time.Time, time.Duration) (domain.InboxClaim, error) {
	return i.claim, i.claimErr
}

func (i *recordingDeliveryInbox) CompleteConsumerMessage(context.Context, string, string, uuid.UUID, uuid.UUID, time.Time) (bool, error) {
	i.completeCall++
	return i.completed, nil
}

func (i *recordingDeliveryInbox) FailConsumerMessage(context.Context, string, string, uuid.UUID, uuid.UUID, time.Time, string) (bool, error) {
	i.failCall++
	return true, nil
}

type recordingJourneySideEffectExecutor struct {
	calls     int
	effectKey string
	err       error
}

type recordingSideEffectRepository struct {
	execution   domain.SideEffectExecution
	transitions []domain.SideEffectStatus
}

func (r *recordingSideEffectRepository) GetSideEffect(context.Context, string, string) (domain.SideEffectExecution, error) {
	return r.execution, nil
}

func (r *recordingSideEffectRepository) TransitionSideEffect(
	_ context.Context, _, _ string, _, to domain.SideEffectStatus, _ time.Time, _ *string,
) (bool, error) {
	r.transitions = append(r.transitions, to)
	r.execution.Status = to
	return true, nil
}

func newRecordingDeliveryWorker(t *testing.T, inbox *recordingDeliveryInbox, executor *recordingJourneySideEffectExecutor) (*RealtimeDeliveryWorker, *recordingSideEffectRepository) {
	t.Helper()
	effects := &recordingSideEffectRepository{execution: domain.SideEffectExecution{
		EffectKey: "effect-1", Channel: "email", Status: domain.SideEffectStatusReserved,
	}}
	worker, err := NewRealtimeDeliveryWorker(inbox, effects, executor, time.Minute)
	require.NoError(t, err)
	return worker, effects
}

func (e *recordingJourneySideEffectExecutor) ExecuteJourneySideEffect(_ context.Context, _ domain.EventEnvelope, effectKey string) error {
	e.calls++
	e.effectKey = effectKey
	return e.err
}

func deliveryMessage(t *testing.T) broker.Message {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	body, err := json.Marshal(domain.EventEnvelope{
		ID: id, EventID: uuid.New(), Type: "journey.side_effect.requested", SchemaVersion: 1,
		WorkspaceID: "workspace-1", Subject: domain.EventSubject{Type: "contact_automation", ID: "ca-1"},
		Source: "journey-worker", OccurredAt: now, ReceivedAt: now, CorrelationID: uuid.New(),
		Data: json.RawMessage(`{"node_type":"email"}`),
	})
	require.NoError(t, err)
	return broker.Message{ID: id, Type: "journey.side_effect.requested", Headers: map[string]any{"effect_key": "effect-1"}, Body: body}
}

func TestRealtimeDeliveryWorkerCompletesInboxAfterEffect(t *testing.T) {
	inbox := &recordingDeliveryInbox{claim: domain.InboxClaim{Acquired: true, ClaimToken: uuid.New()}, completed: true}
	executor := &recordingJourneySideEffectExecutor{}
	worker, effects := newRecordingDeliveryWorker(t, inbox, executor)

	decision := worker.HandleDelivery(context.Background(), deliveryMessage(t))

	assert.Equal(t, broker.Ack, decision.Action)
	assert.Equal(t, 1, executor.calls)
	assert.Equal(t, "effect-1", executor.effectKey)
	assert.Equal(t, 1, inbox.completeCall)
	assert.Zero(t, inbox.failCall)
	assert.Equal(t, []domain.SideEffectStatus{domain.SideEffectStatusSubmitted, domain.SideEffectStatusConfirmed}, effects.transitions)
}

func TestRealtimeDeliveryWorkerAcknowledgesCompletedDuplicate(t *testing.T) {
	inbox := &recordingDeliveryInbox{claim: domain.InboxClaim{Status: domain.InboxStatusCompleted}}
	executor := &recordingJourneySideEffectExecutor{}
	worker, _ := newRecordingDeliveryWorker(t, inbox, executor)

	decision := worker.HandleDelivery(context.Background(), deliveryMessage(t))

	assert.Equal(t, broker.Ack, decision.Action)
	assert.Zero(t, executor.calls)
}

func TestRealtimeDeliveryWorkerRetriesFailedEffect(t *testing.T) {
	inbox := &recordingDeliveryInbox{claim: domain.InboxClaim{Acquired: true, ClaimToken: uuid.New()}}
	executor := &recordingJourneySideEffectExecutor{err: errors.New("provider timeout")}
	worker, effects := newRecordingDeliveryWorker(t, inbox, executor)

	decision := worker.HandleDelivery(context.Background(), deliveryMessage(t))

	assert.Equal(t, broker.Retry, decision.Action)
	assert.Equal(t, broker.Retry30Seconds, decision.RetryTier)
	assert.Equal(t, 1, inbox.failCall)
	assert.Equal(t, domain.SideEffectStatusFailed, effects.execution.Status)
}

func TestRealtimeDeliveryWorkerDeadLettersMalformedCommand(t *testing.T) {
	inbox := &recordingDeliveryInbox{}
	executor := &recordingJourneySideEffectExecutor{}
	worker, _ := newRecordingDeliveryWorker(t, inbox, executor)

	decision := worker.HandleDelivery(context.Background(), broker.Message{ID: uuid.New(), Body: []byte(`{}`)})

	assert.Equal(t, broker.DeadLetter, decision.Action)
	assert.ErrorIs(t, decision.Err, ErrInvalidDeliveryMessage)
}

func TestRealtimeDeliveryWorkerDoesNotRepeatSubmittedWebhookAfterCrash(t *testing.T) {
	inbox := &recordingDeliveryInbox{claim: domain.InboxClaim{Acquired: true, ClaimToken: uuid.New()}}
	executor := &recordingJourneySideEffectExecutor{}
	worker, effects := newRecordingDeliveryWorker(t, inbox, executor)
	effects.execution.Channel = "webhook"
	effects.execution.Status = domain.SideEffectStatusSubmitted

	decision := worker.HandleDelivery(context.Background(), deliveryMessage(t))

	assert.Equal(t, broker.DeadLetter, decision.Action)
	assert.ErrorIs(t, decision.Err, ErrSideEffectOutcomeUnknown)
	assert.Zero(t, executor.calls, "an uncertain webhook must never be called twice automatically")
	assert.Equal(t, domain.SideEffectStatusUnknown, effects.execution.Status)
}
