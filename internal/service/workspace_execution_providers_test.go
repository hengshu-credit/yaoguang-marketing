package service

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	mocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type providerSaveHook struct {
	calls int
	saved *bool
}

func (h *providerSaveHook) UpdateWorkspaceProviders(_ context.Context, _ string, save func() error) error {
	h.calls++
	if err := save(); err != nil {
		return err
	}
	if !*h.saved {
		panic("provider archive published before settings were saved")
	}
	return nil
}
func TestWorkspaceProviderEditsPublishFutureExecutionArchives(t *testing.T) {
	for _, kind := range []string{"integration", "default"} {
		t.Run(kind, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mocks.NewMockWorkspaceRepository(ctrl)
			auth := mocks.NewMockAuthService(ctrl)
			auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws").Return(context.Background(), &domain.User{ID: "owner"}, &domain.UserWorkspace{Role: "owner"}, nil)
			workspace := &domain.Workspace{ID: "ws", Name: "Workspace", Settings: domain.WorkspaceSettings{WebsiteURL: "https://example.com", DefaultLanguage: "en", Timezone: "UTC", Languages: []string{"en"}}, Integrations: domain.Integrations{{ID: "sms", Name: "SMS", Type: domain.IntegrationTypeSMS, SMSProvider: &domain.SMSProvider{Kind: domain.SMSProviderKindTwilio, Twilio: &domain.TwilioSettings{AccountSID: "AC" + strings.Repeat("a", 32), AuthToken: "old", FromNumber: "+15551234567"}}}}}
			repo.EXPECT().GetByID(gomock.Any(), "ws").Return(workspace, nil)
			saved := false
			hook := &providerSaveHook{saved: &saved}
			svc := &WorkspaceService{repo: repo, authService: auth, secretKey: "secret", executionSnapshots: hook}
			repo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, w *domain.Workspace) error { saved = true; return nil })
			if kind == "integration" {
				require.NoError(t, svc.UpdateIntegration(context.Background(), domain.UpdateIntegrationRequest{WorkspaceID: "ws", IntegrationID: "sms", Name: "Edited SMS", SMSProvider: &domain.SMSProvider{Kind: domain.SMSProviderKindTwilio, Twilio: &domain.TwilioSettings{AccountSID: "AC" + strings.Repeat("a", 32), AuthToken: "new", FromNumber: "+15551234567"}}}))
			} else {
				settings := workspace.Settings
				settings.MarketingEmailProviderID = "changed-default"
				_, err := svc.UpdateWorkspace(context.Background(), "ws", "Workspace", settings)
				require.NoError(t, err)
			}
			require.True(t, saved)
			require.Equal(t, 1, hook.calls)
		})
	}
}
