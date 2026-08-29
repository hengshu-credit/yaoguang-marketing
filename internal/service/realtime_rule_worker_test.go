package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

type recordingRuleProcessor struct {
	requests []domain.RuleProcessRequest
	result   domain.RuleProcessResult
	err      error
}

func (p *recordingRuleProcessor) ProcessRuleEvent(_ context.Context, request domain.RuleProcessRequest) (domain.RuleProcessResult, error) {
	p.requests = append(p.requests, request)
	return p.result, p.err
}

func TestShadowMatcherRequestsAuditWithoutPrimaryEnrollment(t *testing.T) {
	processor := &recordingRuleProcessor{result: domain.RuleProcessResult{Candidates: 2, Matched: 1}}
	worker, err := NewRuleWorker(processor, config.RealtimeModeShadow, time.Minute)
	require.NoError(t, err)

	require.NoError(t, worker.Handle(context.Background(), ruleEventMessage(t)))
	require.Len(t, processor.requests, 1)
	assert.False(t, processor.requests[0].Primary)
	assert.Equal(t, domain.MatchEngineRealtime, processor.requests[0].Engine)
	assert.Contains(t, processor.requests[0].DependencyKeys, "changes.language")
}

func TestPrimaryMatcherEnablesAtomicEnrollment(t *testing.T) {
	processor := &recordingRuleProcessor{result: domain.RuleProcessResult{Candidates: 1, Matched: 1, Enrolled: 1}}
	worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)

	require.NoError(t, worker.Handle(context.Background(), ruleEventMessage(t)))
	require.Len(t, processor.requests, 1)
	assert.True(t, processor.requests[0].Primary)
	assert.Equal(t, "rule-worker", processor.requests[0].Consumer)
}

func TestRuleWorkerMapsBusyInboxToRetry(t *testing.T) {
	processor := &recordingRuleProcessor{result: domain.RuleProcessResult{Busy: true}}
	worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)

	decision := worker.HandleDelivery(context.Background(), ruleEventMessage(t))
	assert.Equal(t, broker.Retry, decision.Action)
	assert.Equal(t, broker.Retry5Seconds, decision.RetryTier)
}

func TestRuleWorkerAcknowledgesCompletedDuplicate(t *testing.T) {
	processor := &recordingRuleProcessor{result: domain.RuleProcessResult{Duplicate: true}}
	worker, err := NewRuleWorker(processor, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)

	decision := worker.HandleDelivery(context.Background(), ruleEventMessage(t))
	assert.Equal(t, broker.Ack, decision.Action)
}

func ruleEventMessage(t *testing.T) broker.Message {
	t.Helper()
	messageID := uuid.New()
	envelope := domain.EventEnvelope{
		ID:            messageID,
		EventID:       uuid.New(),
		Type:          "contact.updated",
		SchemaVersion: 1,
		WorkspaceID:   "tenant1",
		Subject: domain.EventSubject{
			Type: "contact", ID: "person@example.com", ContactEmail: "person@example.com",
		},
		Source:        "contact_timeline",
		OccurredAt:    time.Now().UTC(),
		ReceivedAt:    time.Now().UTC(),
		CorrelationID: uuid.New(),
		Data: json.RawMessage(`{
			"entity_id":"person@example.com",
			"changes":{"language":{"old":"en","new":"fr"}}
		}`),
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	return broker.Message{ID: messageID, Body: body}
}
