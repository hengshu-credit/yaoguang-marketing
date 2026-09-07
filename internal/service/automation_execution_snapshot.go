package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func WithAutomationExecutionSnapshots(snapshots *ExecutionSnapshotService) AutomationExecutorOption {
	return func(e *AutomationExecutor) { e.executionSnapshots = snapshots }
}
func (e *AutomationExecutor) bindJourneySnapshot(ctx context.Context, ws, contactAutomationID string, current *domain.Automation) (context.Context, *domain.Automation, error) {
	if e.executionSnapshots == nil {
		return ctx, current, nil
	}
	snapshot, err := e.executionSnapshots.GetInternal(ctx, ws, contactAutomationID)
	// Old instances predate enrollment capture. They remain explicitly absent in
	// history; no synthetic historical snapshot is produced from today's config.
	if errors.Is(err, domain.ErrExecutionSnapshotNotFound) {
		return ctx, current, nil
	}
	if err != nil {
		return ctx, nil, err
	}
	var frozen domain.Automation
	if err = json.Unmarshal(snapshot.Configuration, &frozen); err != nil {
		return ctx, nil, err
	}
	if frozen.Trigger == nil {
		var stored struct {
			Trigger *domain.TimelineTriggerConfig `json:"trigger_config"`
		}
		if err = json.Unmarshal(snapshot.Configuration, &stored); err != nil {
			return ctx, nil, err
		}
		frozen.Trigger = stored.Trigger
	}
	if err = e.executionSnapshots.hydrateRuntimeNodes(ctx, ws, snapshot, &frozen); err != nil {
		return ctx, nil, err
	}
	frozen.Status = current.Status
	bound, err := domain.WithExecutionSnapshot(ctx, snapshot)
	if err == nil {
		bound, err = e.executionSnapshots.bindRuntimeProviders(bound, ws, snapshot)
		bound = domain.WithExecutionMediaArchive(bound, e.executionSnapshots.ArchiveRenderedMedia)
	}
	return bound, &frozen, err
}

func (e *AutomationExecutor) claimEvidence(ctx context.Context, ca *domain.ContactAutomation, node *domain.AutomationNode, action domain.NodeAction, output map[string]interface{}) *domain.NodeExecution {
	if domain.ExecutionIDFromContext(ctx) == "" {
		return nil
	}
	evidence := e.createNodeExecution(ca, node, action)
	evidence.Output = map[string]interface{}{"configuration": node.Config, "result": output}
	return evidence
}
