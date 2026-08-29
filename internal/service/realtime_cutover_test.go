package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type recordingTriggerLifecycleRepository struct {
	automations []*domain.Automation
	actions     []string
	bindErrAt   string
}

func (r *recordingTriggerLifecycleRepository) List(context.Context, string, domain.AutomationFilter) ([]*domain.Automation, int, error) {
	return r.automations, len(r.automations), nil
}
func (r *recordingTriggerLifecycleRepository) CreateRealtimeTriggerBinding(_ context.Context, _ string, automation *domain.Automation) error {
	r.actions = append(r.actions, "bind:"+automation.ID)
	if automation.ID == r.bindErrAt {
		return errors.New("invalid binding")
	}
	return nil
}
func (r *recordingTriggerLifecycleRepository) DropLegacyAutomationTrigger(_ context.Context, _, automationID string) error {
	r.actions = append(r.actions, "drop:"+automationID)
	return nil
}
func (r *recordingTriggerLifecycleRepository) CreateAutomationTrigger(_ context.Context, _ string, automation *domain.Automation) error {
	r.actions = append(r.actions, "restore:"+automation.ID)
	return nil
}

func TestRealtimeCutoverValidatesEveryBindingBeforeDroppingAnyTrigger(t *testing.T) {
	repo := &recordingTriggerLifecycleRepository{
		automations: []*domain.Automation{{ID: "a-1"}, {ID: "a-2"}}, bindErrAt: "a-2",
	}
	service, err := NewRealtimeCutoverService(repo)
	require.NoError(t, err)

	_, err = service.ActivatePrimaryWorkspace(context.Background(), "workspace-1", PrimaryCutoverAssessment{Ready: true})

	require.ErrorContains(t, err, "prepare realtime binding a-2")
	assert.Equal(t, []string{"bind:a-1", "bind:a-2"}, repo.actions)
}

func TestRealtimeCutoverDropsLegacyTriggersOnlyAfterAllBindingsReady(t *testing.T) {
	repo := &recordingTriggerLifecycleRepository{
		automations: []*domain.Automation{{ID: "a-1"}, {ID: "a-2"}},
	}
	service, err := NewRealtimeCutoverService(repo)
	require.NoError(t, err)

	report, err := service.ActivatePrimaryWorkspace(context.Background(), "workspace-1", PrimaryCutoverAssessment{Ready: true})

	require.NoError(t, err)
	assert.Equal(t, []string{"bind:a-1", "bind:a-2", "drop:a-1", "drop:a-2"}, repo.actions)
	assert.Equal(t, 2, report.BindingsPrepared)
	assert.Equal(t, 2, report.LegacyTriggersDropped)
}

func TestRealtimeCutoverRollbackRestoresValidatedLegacyTriggers(t *testing.T) {
	repo := &recordingTriggerLifecycleRepository{
		automations: []*domain.Automation{{ID: "a-1"}, {ID: "a-2"}},
	}
	service, err := NewRealtimeCutoverService(repo)
	require.NoError(t, err)

	report, err := service.RestoreLegacyWorkspace(context.Background(), "workspace-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"restore:a-1", "restore:a-2"}, repo.actions)
	assert.Equal(t, 2, report.LegacyTriggersRestored)
}
