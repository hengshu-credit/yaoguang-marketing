package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
	"github.com/stretchr/testify/require"
)

type recordingStateProjector struct {
	events []domain.EventEnvelope
	err    error
}

func (p *recordingStateProjector) ProcessEvent(_ context.Context, _ string, e domain.EventEnvelope) error {
	p.events = append(p.events, e)
	return p.err
}

func configurableRuleMessage(t *testing.T, status string, expires *time.Time) broker.Message {
	t.Helper()
	e := domain.EventEnvelope{ID: uuid.New(), EventID: uuid.New(), Type: "contract.signed", SchemaVersion: 1, WorkspaceID: "workspace", Subject: domain.EventSubject{Type: "contract", ID: uuid.NewString(), CustomerID: uuid.NewString()}, Source: "backend", OccurredAt: time.Now(), ReceivedAt: time.Now(), CorrelationID: uuid.New()}
	e.Data, _ = json.Marshal(map[string]any{"report_id": "event-1", "processing_status": status, "expires_at": expires, "data": map[string]any{}})
	body, err := json.Marshal(e)
	require.NoError(t, err)
	return broker.Message{ID: e.ID, Body: body}
}

func TestConfiguredRuleAllowsCustomerUUIDWithoutEmail(t *testing.T) {
	processor := &recordingRuleProcessor{}
	projector := &recordingStateProjector{}
	worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)
	worker.SetStateProjector(projector)
	require.NoError(t, worker.Handle(context.Background(), configurableRuleMessage(t, "accepted", nil)))
	require.Len(t, projector.events, 1)
	require.Len(t, processor.requests, 1)
	require.True(t, processor.requests[0].Primary)
}

func TestConfiguredRuleProjectsExpiredHistoryWithoutTriggering(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	for _, tc := range []struct {
		status  string
		expires *time.Time
	}{{"expired", nil}, {"backfill", nil}, {"accepted", &past}} {
		t.Run(tc.status, func(t *testing.T) {
			processor := &recordingRuleProcessor{}
			projector := &recordingStateProjector{}
			worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
			require.NoError(t, err)
			worker.SetStateProjector(projector)
			require.NoError(t, worker.Handle(context.Background(), configurableRuleMessage(t, tc.status, tc.expires)))
			require.Len(t, projector.events, 1)
			require.Empty(t, processor.requests)
		})
	}
}

func TestConfiguredRuleRetriesFailedProjectionBeforeSending(t *testing.T) {
	processor := &recordingRuleProcessor{}
	projector := &recordingStateProjector{err: errors.New("projection unavailable")}
	worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)
	worker.SetStateProjector(projector)
	decision := worker.HandleDelivery(context.Background(), configurableRuleMessage(t, "accepted", nil))
	require.Equal(t, broker.Retry, decision.Action)
	require.Empty(t, processor.requests)
}

func TestConfiguredRuleAcknowledgesUnresolvedHistoricalOccurrence(t *testing.T) {
	processor := &recordingRuleProcessor{}
	worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)
	message := configurableRuleMessage(t, "expired", nil)
	var event domain.EventEnvelope
	require.NoError(t, json.Unmarshal(message.Body, &event))
	event.Subject = domain.EventSubject{Type: "unresolved", ID: event.EventID.String()}
	message.Body, err = json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, worker.Handle(context.Background(), message))
	require.Empty(t, processor.requests)
}
