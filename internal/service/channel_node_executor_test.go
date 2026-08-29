package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingChannelMessageService struct {
	request  *domain.SendChannelMessageRequest
	response *domain.SendChannelMessageResponse
	err      error
}

func (s *recordingChannelMessageService) Send(_ context.Context, request *domain.SendChannelMessageRequest) (*domain.SendChannelMessageResponse, error) {
	s.request = request
	return s.response, s.err
}

func TestChannelNodeExecutorUsesRealtimeEffectKey(t *testing.T) {
	next := "next-node"
	channelService := &recordingChannelMessageService{response: &domain.SendChannelMessageResponse{
		Execution: domain.ChannelSendExecution{Status: domain.ChannelSendConfirmed, ProviderMessageID: "SM123"},
	}}
	executor := NewChannelNodeExecutor(domain.NodeTypeSMS, channelService)
	result, err := executor.Execute(context.Background(), NodeExecutionParams{
		WorkspaceID: "ws-1",
		Contact:     &domain.ContactAutomation{ID: "ca-1", ContactEmail: "user@example.com"},
		Automation:  &domain.Automation{ID: "automation-1", Name: "Order ready"},
		Node: &domain.AutomationNode{ID: "sms-1", Type: domain.NodeTypeSMS, NextNodeID: &next, Config: map[string]interface{}{
			"template_id": "ready", "integration_id": "twilio-main", "endpoint_id": "phone-primary",
		}},
		ExecutionContext: map[string]interface{}{realtimeEffectKeyContext: "journey:effect-1"},
	})
	require.NoError(t, err)
	assert.Equal(t, &next, result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	require.NotNil(t, channelService.request)
	assert.Equal(t, "journey:effect-1", channelService.request.EffectKey)
	assert.Equal(t, domain.ChannelSMS, channelService.request.Channel)
	assert.Equal(t, "phone-primary", channelService.request.EndpointID)
	assert.Equal(t, "automation-1", channelService.request.Data["automation_id"])
}

func TestChannelNodeExecutorRejectsNonConfirmedDuplicate(t *testing.T) {
	channelService := &recordingChannelMessageService{response: &domain.SendChannelMessageResponse{
		Duplicate: true, Execution: domain.ChannelSendExecution{Status: domain.ChannelSendUnknown},
	}}
	executor := NewChannelNodeExecutor(domain.NodeTypePush, channelService)
	_, err := executor.Execute(context.Background(), NodeExecutionParams{
		WorkspaceID: "ws-1",
		Contact:     &domain.ContactAutomation{ID: "ca-1", ContactEmail: "user@example.com"},
		Automation:  &domain.Automation{ID: "automation-1"},
		Node: &domain.AutomationNode{ID: "push-1", Type: domain.NodeTypePush, Config: map[string]interface{}{
			"template_id": "ready", "integration_id": "fcm-main",
		}},
	})
	assert.ErrorContains(t, err, "channel send effect is in unknown state")
	assert.ErrorIs(t, err, ErrSideEffectOutcomeUnknown)
}
