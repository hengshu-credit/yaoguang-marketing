package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationNodeReferenceValidatorRejectsTemplateChannelOnSave(t *testing.T) {
	ctrl := gomock.NewController(t)
	templates := mocks.NewMockTemplateService(ctrl)
	workspaces := mocks.NewMockWorkspaceRepository(ctrl)
	validator := NewAutomationNodeReferenceValidator(templates, workspaces)
	templates.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "push-template", int64(4)).Return(&domain.Template{
		ID: "push-template", Version: 4, Channel: "push", Push: &domain.PushTemplate{Title: "Hi", Body: "Body"},
	}, nil)

	err := validator.ValidateAutomationNodes(context.Background(), "ws1", &domain.Automation{Nodes: []*domain.AutomationNode{{
		ID: "sms-node-1", Type: domain.NodeTypeSMS, Config: map[string]interface{}{
			"template_id": "push-template", "template_version": 4, "integration_id": "twilio-main",
		},
	}}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "automation node sms-node-1 field template_id")
	assert.ErrorContains(t, err, "expected sms")
}

func TestAutomationNodeReferenceValidatorRejectsWrongProviderTypeOnSave(t *testing.T) {
	ctrl := gomock.NewController(t)
	templates := mocks.NewMockTemplateService(ctrl)
	workspaces := mocks.NewMockWorkspaceRepository(ctrl)
	validator := NewAutomationNodeReferenceValidator(templates, workspaces)
	templates.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "sms-template", int64(0)).Return(&domain.Template{
		ID: "sms-template", Version: 1, Channel: "sms", SMS: &domain.SMSTemplate{Body: "Hi"},
	}, nil)
	workspaces.EXPECT().GetByID(gomock.Any(), "ws1").Return(&domain.Workspace{Integrations: domain.Integrations{{
		ID: "fcm-main", Name: "Push", Type: domain.IntegrationTypePush,
	}}}, nil)

	err := validator.ValidateAutomationNodes(context.Background(), "ws1", &domain.Automation{Nodes: []*domain.AutomationNode{{
		ID: "sms-node-1", Type: domain.NodeTypeSMS, Config: map[string]interface{}{
			"template_id": "sms-template", "integration_id": "fcm-main",
		},
	}}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "automation node sms-node-1 field integration_id")
	assert.ErrorContains(t, err, "not a sms Provider")
}
