package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

type journeyPreflightSourceStub struct {
	snapshot *domain.JourneyPreflightSnapshot
}

func (s journeyPreflightSourceStub) LoadJourneyPreflightSnapshot(context.Context, string, string) (*domain.JourneyPreflightSnapshot, error) {
	return s.snapshot, nil
}

func TestJourneyPreflightClassifiesGraphContentAndGovernance(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	n2 := "send"
	automation := &domain.Automation{
		ID: "journey-1", WorkspaceID: "ws", Name: "Activation", Status: domain.AutomationStatusDraft,
		Trigger:    &domain.TimelineTriggerConfig{EventKind: "custom_event", CustomEventName: stringPtr("loan.approved"), Frequency: domain.TriggerFrequencyEveryTime},
		RootNodeID: "start", UpdatedAt: now,
		Nodes: []*domain.AutomationNode{
			{ID: "start", AutomationID: "journey-1", Type: domain.NodeTypeTrigger, Config: map[string]interface{}{}, NextNodeID: &n2},
			{ID: "send", AutomationID: "journey-1", Type: domain.NodeTypeSMS, Config: map[string]interface{}{"template_id": "tpl", "integration_id": "sms"}},
			{ID: "orphan", AutomationID: "journey-1", Type: domain.NodeTypeWebhook, Config: map[string]interface{}{"url": "https://example.test/hook"}},
		},
	}
	service, err := NewJourneyPreflightService(journeyPreflightSourceStub{snapshot: &domain.JourneyPreflightSnapshot{
		Automation:         automation,
		TemplateChecks:     []domain.JourneyTemplateCheck{{NodeID: "send", Channel: "sms", TemplateID: "tpl", Exists: true, ChannelMatches: false, ProviderReady: false}},
		VariableErrors:     map[string][]string{"send": {"customer.mobile is missing a default"}},
		HasFrequencyPolicy: false,
	}}, nil, func() time.Time { return now })
	require.NoError(t, err)

	result, err := service.PreflightAutomation(context.Background(), domain.JourneyPreflightRequest{WorkspaceID: "ws", AutomationID: "journey-1"})
	require.NoError(t, err)
	codes := make(map[string]domain.JourneyPreflightSeverity)
	for _, issue := range result.Issues {
		codes[issue.Code] = issue.Severity
	}
	require.Equal(t, domain.JourneyPreflightBlocking, codes["node_unreachable"])
	require.Equal(t, domain.JourneyPreflightBlocking, codes["template_channel_mismatch"])
	require.Equal(t, domain.JourneyPreflightBlocking, codes["provider_missing"])
	require.Equal(t, domain.JourneyPreflightWarning, codes["variable_sample_failed"])
	require.Equal(t, domain.JourneyPreflightWarning, codes["frequency_policy_missing"])
	require.Equal(t, domain.JourneyPreflightWarning, codes["webhook_secret_missing"])
	require.NotEmpty(t, result.SummaryHash)
}

func TestJourneyPreflightBlocksCyclesAndRequiresWarningConfirmation(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	n2, n1 := "n2", "n1"
	automation := &domain.Automation{
		ID: "journey-1", WorkspaceID: "ws", Name: "cycle", Status: domain.AutomationStatusDraft,
		Trigger:    &domain.TimelineTriggerConfig{EventKind: "contact.created", Frequency: domain.TriggerFrequencyOnce},
		RootNodeID: "n1", UpdatedAt: now,
		Nodes: []*domain.AutomationNode{
			{ID: "n1", AutomationID: "journey-1", Type: domain.NodeTypeTrigger, Config: map[string]interface{}{}, NextNodeID: &n2},
			{ID: "n2", AutomationID: "journey-1", Type: domain.NodeTypeDelay, Config: map[string]interface{}{"duration": 1, "unit": "days"}, NextNodeID: &n1},
		},
	}
	source := journeyPreflightSourceStub{snapshot: &domain.JourneyPreflightSnapshot{Automation: automation, HasFrequencyPolicy: true}}
	service, err := NewJourneyPreflightService(source, nil, func() time.Time { return now })
	require.NoError(t, err)
	result, err := service.PreflightAutomation(context.Background(), domain.JourneyPreflightRequest{WorkspaceID: "ws", AutomationID: "journey-1"})
	require.NoError(t, err)
	require.Positive(t, result.BlockingCount)
	require.ErrorIs(t, service.ValidateAutomationPreflight(context.Background(), domain.JourneyPreflightRequest{WorkspaceID: "ws", AutomationID: "journey-1"}, result.SummaryHash, true), domain.ErrJourneyPreflightBlocked)
}

func TestJourneyPreflightRequiresExplicitWarningConfirmation(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	automation := &domain.Automation{
		ID: "journey-warning", WorkspaceID: "ws", Name: "webhook", Status: domain.AutomationStatusDraft,
		Trigger:    &domain.TimelineTriggerConfig{EventKind: "contact.created", Frequency: domain.TriggerFrequencyOnce},
		RootNodeID: "hook", UpdatedAt: now,
		Nodes: []*domain.AutomationNode{{ID: "hook", AutomationID: "journey-warning", Type: domain.NodeTypeWebhook, Config: map[string]interface{}{"url": "https://example.test/hook"}}},
	}
	service, err := NewJourneyPreflightService(journeyPreflightSourceStub{snapshot: &domain.JourneyPreflightSnapshot{Automation: automation, HasFrequencyPolicy: true}}, nil, func() time.Time { return now })
	require.NoError(t, err)
	request := domain.JourneyPreflightRequest{WorkspaceID: "ws", AutomationID: automation.ID}
	result, err := service.PreflightAutomation(context.Background(), request)
	require.NoError(t, err)
	require.Zero(t, result.BlockingCount)
	require.Equal(t, 1, result.WarningCount)
	require.ErrorIs(t, service.ValidateAutomationPreflight(context.Background(), request, result.SummaryHash, false), domain.ErrJourneyPreflightWarningConfirmation)
	require.NoError(t, service.ValidateAutomationPreflight(context.Background(), request, result.SummaryHash, true))
}

func stringPtr(value string) *string { return &value }
