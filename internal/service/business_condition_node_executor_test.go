package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type businessConditionStub struct {
	matched bool
	starts  []time.Time
}

func (s *businessConditionStub) EvaluateJourneyBusinessCondition(_ context.Context, _, _ string, _ domain.BusinessConditionNodeConfig, start, _ time.Time) (bool, map[string]any, error) {
	s.starts = append(s.starts, start)
	return s.matched, map[string]any{"same_object": true}, nil
}
func TestBusinessConditionWaitPersistsStartAndExitsOnTimeout(t *testing.T) {
	repo := &businessConditionStub{}
	now := time.Now().UTC()
	executor := NewBusinessConditionNodeExecutor(repo)
	executor.now = func() time.Time { return now }
	config := map[string]any{"mode": "wait", "subject_type": "contract", "events": []string{"signed"}, "timeout_seconds": 60, "on_timeout": "exit"}
	node := &domain.AutomationNode{ID: "wait-node", Type: domain.NodeTypeBusinessCondition, Config: config}
	params := NodeExecutionParams{WorkspaceID: "ws", Node: node, Contact: &domain.ContactAutomation{ID: "journey", Context: map[string]any{"customer_id": "customer"}}}
	waiting, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, "wait-node", *waiting.NextNodeID)
	require.NotNil(t, waiting.ScheduledAt)
	params.Contact.Context = waiting.Context
	now = now.Add(61 * time.Second)
	timedout, err := executor.Execute(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, domain.ContactAutomationStatusExited, timedout.Status)
	require.Equal(t, repo.starts[0], repo.starts[1])
	require.Equal(t, "customer", timedout.Context["customer_id"])
}
func TestBusinessConditionMatchedEventContinuesOrExits(t *testing.T) {
	repo := &businessConditionStub{matched: true}
	executor := NewBusinessConditionNodeExecutor(repo)
	for _, mode := range []string{"wait", "exit_if", "continue_if"} {
		next := "send"
		params := NodeExecutionParams{Node: &domain.AutomationNode{ID: "condition", NextNodeID: &next, Config: map[string]any{"mode": mode, "subject_type": "customer", "events": []string{"login"}, "timeout_seconds": 60, "on_timeout": "exit"}}, Contact: &domain.ContactAutomation{ID: "journey"}}
		got, err := executor.Execute(context.Background(), params)
		require.NoError(t, err)
		if mode == "exit_if" {
			require.Equal(t, domain.ContactAutomationStatusExited, got.Status)
		} else {
			require.Equal(t, &next, got.NextNodeID)
		}
	}
}
