package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ChannelNodeExecutor struct {
	nodeType domain.NodeType
	channel  string
	service  domain.ChannelMessageService
}

func NewChannelNodeExecutor(nodeType domain.NodeType, service domain.ChannelMessageService) *ChannelNodeExecutor {
	channel := domain.ChannelSMS
	if nodeType == domain.NodeTypePush {
		channel = domain.ChannelPush
	}
	return &ChannelNodeExecutor{nodeType: nodeType, channel: channel, service: service}
}

func (e *ChannelNodeExecutor) NodeType() domain.NodeType { return e.nodeType }

func (e *ChannelNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	if params.Contact == nil || params.Automation == nil || params.Node == nil {
		return nil, fmt.Errorf("contact, automation, and node are required for channel node")
	}
	config, err := parseChannelNodeConfig(params.Node.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid %s node config: %w", e.channel, err)
	}
	effectKey, _ := params.ExecutionContext[realtimeEffectKeyContext].(string)
	if effectKey == "" {
		effectKey = fmt.Sprintf("journey:%s:%s:%s:%s", params.WorkspaceID, params.Contact.ID, params.Node.ID, e.channel)
	}
	data := make(domain.MapOfAny, len(config.Data)+2)
	for key, value := range config.Data {
		data[key] = value
	}
	data["automation_id"] = params.Automation.ID
	data["automation_name"] = params.Automation.Name
	response, err := e.service.Send(systemContext(ctx), &domain.SendChannelMessageRequest{
		WorkspaceID: params.WorkspaceID, EffectKey: effectKey, Channel: e.channel,
		IntegrationID: config.IntegrationID, ContactEmail: params.Contact.ContactEmail,
		EndpointID: config.EndpointID, TemplateID: config.TemplateID, Language: config.Language,
		Data: data, Metadata: domain.MapOfAny{"automation_id": params.Automation.ID, "automation_node_id": params.Node.ID},
	})
	if err != nil {
		return nil, err
	}
	if response == nil || response.Execution.Status != domain.ChannelSendConfirmed {
		status := "missing"
		if response != nil {
			status = string(response.Execution.Status)
			if response.Execution.Status == domain.ChannelSendUnknown {
				return nil, fmt.Errorf("%w: channel send effect is in %s state", ErrSideEffectOutcomeUnknown, status)
			}
		}
		return nil, fmt.Errorf("channel send effect is in %s state", status)
	}
	return &NodeExecutionResult{
		NextNodeID: params.Node.NextNodeID, Status: domain.ContactAutomationStatusActive,
		Output: buildNodeOutput(e.nodeType, map[string]interface{}{
			"template_id": config.TemplateID, "integration_id": config.IntegrationID,
			"message_id": response.Execution.MessageID, "provider_message_id": response.Execution.ProviderMessageID,
			"duplicate": response.Duplicate,
		}),
	}, nil
}

func parseChannelNodeConfig(config map[string]interface{}) (*domain.ChannelNodeConfig, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal channel node config: %w", err)
	}
	var parsed domain.ChannelNodeConfig
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, fmt.Errorf("decode channel node config: %w", err)
	}
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	return &parsed, nil
}
