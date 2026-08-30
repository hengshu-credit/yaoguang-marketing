package service

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type AutomationNodeReferenceValidator struct {
	templates  domain.TemplateService
	workspaces domain.WorkspaceRepository
}

func NewAutomationNodeReferenceValidator(templates domain.TemplateService, workspaces domain.WorkspaceRepository) *AutomationNodeReferenceValidator {
	return &AutomationNodeReferenceValidator{templates: templates, workspaces: workspaces}
}

func (v *AutomationNodeReferenceValidator) ValidateAutomationNodes(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	if v == nil || v.templates == nil || v.workspaces == nil || automation == nil {
		return fmt.Errorf("automation node validator is not configured")
	}
	var workspace *domain.Workspace
	loadWorkspace := func() (*domain.Workspace, error) {
		if workspace != nil {
			return workspace, nil
		}
		loaded, err := v.workspaces.GetByID(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		workspace = loaded
		return workspace, nil
	}
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		expectedChannel := ""
		templateID := ""
		templateVersion := int64(0)
		integrationID := ""
		switch node.Type {
		case domain.NodeTypeEmail:
			config, err := parseEmailNodeConfig(node.Config)
			if err != nil {
				return nodeFieldError(node.ID, "template_id", err)
			}
			expectedChannel, templateID, templateVersion = "email", config.TemplateID, config.TemplateVersion
			if config.IntegrationID != nil {
				integrationID = *config.IntegrationID
			}
		case domain.NodeTypeSMS, domain.NodeTypePush:
			config, err := parseChannelNodeConfig(node.Config)
			if err != nil {
				return nodeFieldError(node.ID, "template_id", err)
			}
			expectedChannel, templateID, templateVersion, integrationID = string(node.Type), config.TemplateID, config.TemplateVersion, config.IntegrationID
		default:
			continue
		}
		template, err := v.templates.GetTemplateByID(systemContext(ctx), workspaceID, templateID, templateVersion)
		if err != nil {
			return nodeFieldError(node.ID, "template_id", fmt.Errorf("template %s version %d is unavailable: %w", templateID, templateVersion, err))
		}
		if template.Channel != expectedChannel {
			return nodeFieldError(node.ID, "template_id", fmt.Errorf("template %s version %d is channel %s, expected %s", template.ID, template.Version, template.Channel, expectedChannel))
		}
		workspace, err := loadWorkspace()
		if err != nil {
			return nodeFieldError(node.ID, "integration_id", fmt.Errorf("load workspace Providers: %w", err))
		}
		if integrationID == "" && expectedChannel == "email" {
			if _, _, err := workspace.GetEmailProviderWithIntegrationID(true); err != nil {
				return nodeFieldError(node.ID, "integration_id", fmt.Errorf("workspace default email Provider is unavailable: %w", err))
			}
			continue
		}
		integration := workspace.GetIntegrationByID(integrationID)
		if integration == nil || string(integration.Type) != expectedChannel {
			return nodeFieldError(node.ID, "integration_id", fmt.Errorf("integration %s is not a %s Provider", integrationID, expectedChannel))
		}
	}
	return nil
}

func nodeFieldError(nodeID, field string, err error) error {
	return domain.NewValidationError(fmt.Sprintf("automation node %s field %s: %v", nodeID, field, err))
}

var _ domain.AutomationNodeValidator = (*AutomationNodeReferenceValidator)(nil)
