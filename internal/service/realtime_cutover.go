package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type RealtimeTriggerLifecycleRepository interface {
	List(context.Context, string, domain.AutomationFilter) ([]*domain.Automation, int, error)
	CreateRealtimeTriggerBinding(context.Context, string, *domain.Automation) error
	DropLegacyAutomationTrigger(context.Context, string, string) error
	CreateAutomationTrigger(context.Context, string, *domain.Automation) error
}

type RealtimeCutoverReport = domain.RealtimeCutoverReport

type RealtimeCutoverService struct {
	repository RealtimeTriggerLifecycleRepository
}

func NewRealtimeCutoverService(repository RealtimeTriggerLifecycleRepository) (*RealtimeCutoverService, error) {
	if repository == nil {
		return nil, errors.New("realtime trigger lifecycle repository is required")
	}
	return &RealtimeCutoverService{repository: repository}, nil
}

// ActivatePrimaryWorkspace performs two phases: validate/backfill every live
// binding, then remove legacy triggers. A bad automation therefore leaves all
// legacy enrollment paths intact.
func (s *RealtimeCutoverService) ActivatePrimaryWorkspace(
	ctx context.Context,
	workspaceID string,
	assessment PrimaryCutoverAssessment,
) (RealtimeCutoverReport, error) {
	report := RealtimeCutoverReport{WorkspaceID: workspaceID}
	if workspaceID == "" {
		return report, errors.New("workspace id is required")
	}
	if !assessment.Ready {
		return report, fmt.Errorf("%w: %s", domain.ErrRealtimeCutoverBlocked, strings.Join(assessment.Blockers, "; "))
	}
	automations, _, err := s.repository.List(ctx, workspaceID, domain.AutomationFilter{
		Status: []domain.AutomationStatus{domain.AutomationStatusLive},
	})
	if err != nil {
		return report, fmt.Errorf("list live automations: %w", err)
	}
	for _, automation := range automations {
		if err := s.repository.CreateRealtimeTriggerBinding(ctx, workspaceID, automation); err != nil {
			return report, fmt.Errorf("prepare realtime binding %s: %w", automation.ID, err)
		}
		report.BindingsPrepared++
	}
	for _, automation := range automations {
		if err := s.repository.DropLegacyAutomationTrigger(ctx, workspaceID, automation.ID); err != nil {
			return report, fmt.Errorf("drop legacy trigger %s: %w", automation.ID, err)
		}
		report.LegacyTriggersDropped++
	}
	return report, nil
}

// RestoreLegacyWorkspace is rollback-safe: each legacy trigger is compiled,
// condition-probed and installed transactionally by the repository.
func (s *RealtimeCutoverService) RestoreLegacyWorkspace(
	ctx context.Context,
	workspaceID string,
) (RealtimeCutoverReport, error) {
	report := RealtimeCutoverReport{WorkspaceID: workspaceID}
	if workspaceID == "" {
		return report, errors.New("workspace id is required")
	}
	automations, _, err := s.repository.List(ctx, workspaceID, domain.AutomationFilter{
		Status: []domain.AutomationStatus{domain.AutomationStatusLive},
	})
	if err != nil {
		return report, fmt.Errorf("list live automations: %w", err)
	}
	for _, automation := range automations {
		if err := s.repository.CreateAutomationTrigger(ctx, workspaceID, automation); err != nil {
			return report, fmt.Errorf("restore legacy trigger %s: %w", automation.ID, err)
		}
		report.LegacyTriggersRestored++
	}
	return report, nil
}
