package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	domainmocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	pkgmocks "github.com/hengshu-credit/yaoguang-marketing/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewTemplateGenericChannelUsesTranslationAndRTLDirection(t *testing.T) {
	ctrl := gomock.NewController(t)
	templateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	auth := domainmocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	service := NewTemplateService(templateRepo, workspaceRepo, auth, log, "https://api.example.com")
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "workspace-1").Return(
		ctx,
		&domain.User{ID: "user-1"},
		&domain.UserWorkspace{WorkspaceID: "workspace-1", Permissions: domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true}}},
		nil,
	)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-1").Return(&domain.Workspace{
		ID: "workspace-1",
		Settings: domain.WorkspaceSettings{
			DefaultLanguage: "en",
			Languages:       []string{"en", "ur"},
			WebsiteURL:      "https://shop.example.com/",
		},
	}, nil)

	response, err := service.PreviewTemplate(ctx, domain.PreviewTemplateRequest{
		WorkspaceID:          "workspace-1",
		Channel:              "telegram",
		ContentSchemaVersion: domain.ChannelTemplateContentSchemaVersion,
		Content:              &domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "Welcome {{ customer.name }}"},
		Translations: map[string]domain.TemplateTranslation{
			"ur": {Content: &domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "خوش آمدید {{ customer.name }}"}},
		},
		Language: "ur",
		Profile:  "telegram_mobile",
		TestData: domain.MapOfAny{"customer": domain.MapOfAny{"name": "Ali"}},
	})

	require.NoError(t, err)
	require.NotNil(t, response.ChannelPreview)
	assert.Equal(t, "ur", response.ResolvedLanguage)
	assert.False(t, response.FallbackUsed)
	assert.Equal(t, "rtl", response.ChannelPreview.Direction)
	assert.Equal(t, "خوش آمدید Ali", response.ChannelPreview.Message.Body)
	workspace, ok := response.TestData["workspace"].(domain.MapOfAny)
	require.True(t, ok)
	assert.Equal(t, "https://shop.example.com", workspace["website_url"])
}
