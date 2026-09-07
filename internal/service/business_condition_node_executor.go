package service

import (
	"context"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type BusinessConditionNodeExecutor struct {
	repository domain.JourneyBusinessConditionRepository
	now        func() time.Time
}

func NewBusinessConditionNodeExecutor(repo domain.JourneyBusinessConditionRepository) *BusinessConditionNodeExecutor {
	return &BusinessConditionNodeExecutor{repo, func() time.Time { return time.Now().UTC() }}
}
func (e *BusinessConditionNodeExecutor) NodeType() domain.NodeType {
	return domain.NodeTypeBusinessCondition
}
func WithBusinessConditionNodes(repo domain.JourneyBusinessConditionRepository) AutomationExecutorOption {
	return func(e *AutomationExecutor) {
		e.nodeExecutors[domain.NodeTypeBusinessCondition] = NewBusinessConditionNodeExecutor(repo)
		e.journeyStartGate, _ = repo.(configuredJourneyStartGate)
	}
}
func (e *BusinessConditionNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	if e.repository == nil || params.Contact == nil || params.Node == nil {
		return nil, fmt.Errorf("business condition requires journey and repository")
	}
	c, err := domain.ParseBusinessCondition(params.Node.Config)
	if err != nil {
		return nil, err
	}
	now := e.now()
	start := now
	key := "business_condition:" + params.Node.ID
	if c.Mode != "wait" && !params.Contact.EnteredAt.IsZero() {
		start = params.Contact.EnteredAt
	}
	contextCopy := map[string]any{}
	for k, v := range params.Contact.Context {
		contextCopy[k] = v
	}
	if raw, ok := contextCopy[key].(string); ok {
		if start, err = time.Parse(time.RFC3339Nano, raw); err != nil {
			return nil, fmt.Errorf("invalid persisted wait start: %w", err)
		}
	}
	matched, evidence, err := e.repository.EvaluateJourneyBusinessCondition(ctx, params.WorkspaceID, params.Contact.ID, c, start, now)
	if err != nil {
		return nil, err
	}
	result := &NodeExecutionResult{NextNodeID: params.Node.NextNodeID, Status: domain.ContactAutomationStatusActive, Context: contextCopy, Output: buildNodeOutput(domain.NodeTypeBusinessCondition, evidence)}
	result.Output["matched"] = matched
	result.Output["mode"] = c.Mode
	result.Output["started_at"] = start
	exit := func(reason string) {
		result.NextNodeID = nil
		result.Status = domain.ContactAutomationStatusExited
		result.ExitReason = &reason
	}
	switch c.Mode {
	case "wait":
		deadline := start.Add(time.Duration(c.TimeoutSeconds) * time.Second)
		result.Output["deadline"] = deadline
		if !matched && now.Before(deadline) {
			result.NextNodeID = &params.Node.ID
			next := now.Add(5 * time.Second)
			if next.After(deadline) {
				next = deadline
			}
			result.ScheduledAt = &next
			contextCopy[key] = start.Format(time.RFC3339Nano)
			result.Output["waiting"] = true
			return result, nil
		}
		if !matched {
			result.Output["timed_out"] = true
			if c.OnTimeout == "exit" {
				exit("business_condition_timeout")
			}
		}
	case "exit_if":
		if matched {
			exit("business_condition_matched")
		}
	case "continue_if":
		if !matched {
			exit("business_condition_not_matched")
		}
	}
	delete(contextCopy, key)
	return result, nil
}
