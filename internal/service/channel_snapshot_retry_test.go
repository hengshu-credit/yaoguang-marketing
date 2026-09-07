package service

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	domainmocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

type recoveringEvidence struct {
	domain.ExecutionSnapshotRepository
	failStatus string
	failed     bool
	saved      map[string]domain.ExecutionEvidence
}

func (r *recoveringEvidence) AppendEvidence(_ context.Context, _ string, e domain.ExecutionEvidence) error {
	if e.Status == r.failStatus && !r.failed {
		r.failed = true
		return errors.New("evidence storage unavailable")
	}
	r.saved[e.Key] = e
	return nil
}
func TestChannelMessageServiceEvidenceFailureRecoversWithoutDuplicateSend(t *testing.T) {
	for _, failStatus := range []string{"rendered", "confirmed"} {
		t.Run(failStatus, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			wr := domainmocks.NewMockWorkspaceRepository(ctrl)
			cr := domainmocks.NewMockContactRepository(ctrl)
			ts := domainmocks.NewMockTemplateService(ctrl)
			wr.EXPECT().GetByID(gomock.Any(), "ws").Return(&domain.Workspace{ID: "ws", Settings: domain.WorkspaceSettings{SecretKey: "secret"}}, nil).AnyTimes()
			cr.EXPECT().GetContactByEmail(gomock.Any(), "ws", "a@example.com").Return(&domain.Contact{Email: "a@example.com"}, nil).AnyTimes()
			template := &domain.Template{ID: "t", Version: 1, Channel: domain.ChannelSMS, SMS: &domain.SMSTemplate{Body: "Hello"}}
			ts.EXPECT().GetTemplateByID(gomock.Any(), "ws", "t", gomock.Any()).Return(template, nil).AnyTimes()
			ts.EXPECT().PreviewTemplate(gomock.Any(), gomock.Any()).Return(&domain.PreviewTemplateResponse{SMS: &domain.SMSPreview{Body: "Hello"}, ResolvedLanguage: "en"}, nil).AnyTimes()
			provider := &fakeChannelProvider{result: &domain.ChannelDeliveryResult{Provider: "twilio", ProviderMessageID: "SM1", Status: "queued"}}
			ledger := &fakeChannelSendRepository{created: true}
			evidence := &recoveringEvidence{failStatus: failStatus, saved: map[string]domain.ExecutionEvidence{}}
			svc, err := NewChannelMessageService(ChannelMessageServiceConfig{Auth: domainmocks.NewMockAuthService(ctrl), WorkspaceRepo: wr, ContactRepo: cr, EndpointRepo: &fakeContactEndpointRepository{endpoints: []*domain.ContactEndpoint{{EndpointID: "phone", Channel: domain.ChannelSMS, Enabled: true, Address: "+15551234567"}}}, TemplateService: ts, SendRepo: ledger, ProviderResolver: fakeChannelProviderResolver{provider}})
			require.NoError(t, err)
			svc.executionSnapshots = &ExecutionSnapshotService{repository: evidence}
			snapshot, err := domain.NewExecutionSnapshot("journey", "j", map[string]any{}, []*domain.Template{template})
			require.NoError(t, err)
			ctx, err := domain.WithExecutionSnapshot(context.WithValue(context.Background(), domain.SystemCallKey, true), snapshot)
			require.NoError(t, err)
			req := &domain.SendChannelMessageRequest{WorkspaceID: "ws", EffectKey: "e", Channel: domain.ChannelSMS, IntegrationID: "i", ContactEmail: "a@example.com", TemplateID: "t"}
			_, err = svc.Send(ctx, req)
			require.Error(t, err)
			ledger.created = false
			response, err := svc.Send(ctx, req)
			require.NoError(t, err)
			require.Equal(t, domain.ChannelSendConfirmed, response.Execution.Status)
			require.Equal(t, 1, provider.calls)
			require.Equal(t, "confirmed", evidence.saved["e:confirmed"].Status)
		})
	}
}
