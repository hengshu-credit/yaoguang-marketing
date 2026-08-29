package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/broker"
)

type recordingProjectionStore struct {
	mu      sync.Mutex
	batches [][]domain.EventEnvelope
	err     error
}

func (s *recordingProjectionStore) InsertBatch(_ context.Context, events []domain.EventEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, append([]domain.EventEnvelope(nil), events...))
	return s.err
}

type recordingAnalyticsInbox struct {
	claims        map[uuid.UUID]domain.InboxClaim
	claimErr      error
	completed     []uuid.UUID
	failed        []uuid.UUID
	completeError error
}

func (r *recordingAnalyticsInbox) ClaimConsumerMessage(_ context.Context, _ string, _ string, messageID uuid.UUID, now time.Time, lease time.Duration) (domain.InboxClaim, error) {
	if r.claimErr != nil {
		return domain.InboxClaim{}, r.claimErr
	}
	if claim, ok := r.claims[messageID]; ok {
		return claim, nil
	}
	return domain.InboxClaim{
		Consumer: "analytics-worker", MessageID: messageID, Status: domain.InboxStatusProcessing,
		ClaimToken: uuid.New(), ClaimExpiresAt: now.Add(lease), Acquired: true,
	}, nil
}

func (r *recordingAnalyticsInbox) CompleteConsumerMessage(_ context.Context, _ string, _ string, messageID, _ uuid.UUID, _ time.Time) (bool, error) {
	r.completed = append(r.completed, messageID)
	return r.completeError == nil, r.completeError
}

func (r *recordingAnalyticsInbox) FailConsumerMessage(_ context.Context, _ string, _ string, messageID, _ uuid.UUID, _ time.Time, _ string) (bool, error) {
	r.failed = append(r.failed, messageID)
	return true, nil
}

func TestAnalyticsWorkerBatchesInStableTenantOrderAndCompletesAfterInsert(t *testing.T) {
	store := &recordingProjectionStore{}
	inbox := &recordingAnalyticsInbox{}
	worker, err := NewAnalyticsWorker(store, inbox, time.Minute)
	require.NoError(t, err)

	later := time.Date(2026, 8, 29, 10, 0, 2, 0, time.UTC)
	earlier := later.Add(-time.Second)
	messages := []broker.Message{
		analyticsMessage(t, "workspace-b", later),
		analyticsMessage(t, "workspace-a", later),
		analyticsMessage(t, "workspace-a", earlier),
	}
	decisions := worker.HandleBatch(context.Background(), messages)
	require.Len(t, decisions, 3)
	for _, decision := range decisions {
		assert.Equal(t, broker.Ack, decision.Action)
	}
	require.Len(t, store.batches, 1)
	require.Len(t, store.batches[0], 3)
	assert.Equal(t, "workspace-a", store.batches[0][0].WorkspaceID)
	assert.Equal(t, earlier, store.batches[0][0].OccurredAt)
	assert.Equal(t, "workspace-a", store.batches[0][1].WorkspaceID)
	assert.Equal(t, "workspace-b", store.batches[0][2].WorkspaceID)
	assert.Len(t, inbox.completed, 3)
}

func TestAnalyticsWorkerDoesNotCompleteInboxWhenClickHouseFails(t *testing.T) {
	store := &recordingProjectionStore{err: errors.New("clickhouse unavailable")}
	inbox := &recordingAnalyticsInbox{}
	worker, err := NewAnalyticsWorker(store, inbox, time.Minute)
	require.NoError(t, err)

	message := analyticsMessage(t, "workspace-a", time.Now().UTC())
	decisions := worker.HandleBatch(context.Background(), []broker.Message{message})
	require.Len(t, decisions, 1)
	assert.Equal(t, broker.Retry, decisions[0].Action)
	assert.Empty(t, inbox.completed)
	assert.Equal(t, []uuid.UUID{message.ID}, inbox.failed)
}

func TestAnalyticsWorkerAcknowledgesCompletedDuplicateWithoutInsert(t *testing.T) {
	message := analyticsMessage(t, "workspace-a", time.Now().UTC())
	store := &recordingProjectionStore{}
	inbox := &recordingAnalyticsInbox{claims: map[uuid.UUID]domain.InboxClaim{
		message.ID: {Status: domain.InboxStatusCompleted, MessageID: message.ID, Acquired: false},
	}}
	worker, err := NewAnalyticsWorker(store, inbox, time.Minute)
	require.NoError(t, err)

	decisions := worker.HandleBatch(context.Background(), []broker.Message{message})
	assert.Equal(t, broker.Ack, decisions[0].Action)
	assert.Empty(t, store.batches)
}

func TestAnalyticsBatcherFlushesAtConfiguredSize(t *testing.T) {
	store := &recordingProjectionStore{}
	worker, err := NewAnalyticsWorker(store, &recordingAnalyticsInbox{}, time.Minute)
	require.NoError(t, err)
	batcher, err := NewAnalyticsBatcher(worker, 2, time.Hour)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go batcher.Run(ctx)

	results := make(chan broker.DeliveryDecision, 2)
	for _, workspace := range []string{"workspace-a", "workspace-b"} {
		message := analyticsMessage(t, workspace, time.Now().UTC())
		go func() { results <- batcher.HandleDelivery(ctx, message) }()
	}
	for range 2 {
		decision := <-results
		assert.Equal(t, broker.Ack, decision.Action)
	}
	require.Len(t, store.batches, 1)
	assert.Len(t, store.batches[0], 2)
}

func analyticsMessage(t *testing.T, workspaceID string, occurredAt time.Time) broker.Message {
	t.Helper()
	messageID := uuid.New()
	envelope := domain.EventEnvelope{
		ID: messageID, EventID: uuid.New(), Type: "contact.updated", SchemaVersion: 1,
		WorkspaceID: workspaceID,
		Subject:     domain.EventSubject{Type: "contact", ID: uuid.NewString(), ContactEmail: "person@example.com"},
		Source:      "crm", OccurredAt: occurredAt, ReceivedAt: occurredAt.Add(time.Millisecond),
		CorrelationID: uuid.New(), Data: json.RawMessage(`{"changes":{"language":{"new":"fr"}}}`),
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	return broker.Message{ID: messageID, Body: body}
}
