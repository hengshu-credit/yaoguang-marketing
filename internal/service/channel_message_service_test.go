package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	domainmocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChannelSendRepository struct {
	stored       domain.ChannelSendExecution
	created      bool
	reserveCalls int
	submitted    int
	confirmed    int
	failed       int
	confirmErr   error
}

func (r *fakeChannelSendRepository) Reserve(_ context.Context, _ string, execution domain.ChannelSendExecution) (domain.ChannelSendExecution, bool, error) {
	r.reserveCalls++
	if r.stored.EffectKey == "" {
		r.stored = execution
	}
	return r.stored, r.created, nil
}
func (r *fakeChannelSendRepository) MarkSubmitted(context.Context, string, string, time.Time) (bool, error) {
	r.submitted++
	return true, nil
}
func (r *fakeChannelSendRepository) Confirm(_ context.Context, _, _, provider, providerMessageID, _ string, _ *domain.MessageHistory, _ time.Time) (bool, error) {
	r.confirmed++
	if r.confirmErr != nil {
		return false, r.confirmErr
	}
	r.stored.Status = domain.ChannelSendConfirmed
	r.stored.Provider = provider
	r.stored.ProviderMessageID = providerMessageID
	return true, nil
}

func TestChannelMessageServiceSendMarksUnknownWhenConfirmationPersistenceFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := domainmocks.NewMockAuthService(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := domainmocks.NewMockContactRepository(ctrl)
	templateService := domainmocks.NewMockTemplateService(ctrl)
	workspace := &domain.Workspace{ID: "ws-1", Settings: domain.WorkspaceSettings{SecretKey: "secret", DefaultLanguage: "en"}}
	contact := &domain.Contact{Email: "user@example.com"}
	template := &domain.Template{ID: "ready", Version: 1, Channel: domain.ChannelSMS, SMS: &domain.SMSTemplate{Body: "Ready"}}
	endpoint := &domain.ContactEndpoint{EndpointID: "phone-1", Channel: domain.ChannelSMS, Provider: domain.EndpointProviderTwilio, Platform: domain.EndpointPlatformPhone, Address: "+15557654321", Enabled: true}

	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").Return(context.Background(), nil, &domain.UserWorkspace{Permissions: domain.UserPermissions{
		domain.PermissionResourceTransactional: {Write: true},
	}}, nil)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(workspace, nil)
	contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws-1", contact.Email).Return(contact, nil)
	templateService.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", template.ID, int64(0)).Return(template, nil)
	templateService.EXPECT().PreviewTemplate(gomock.Any(), gomock.Any()).Return(&domain.PreviewTemplateResponse{
		ResolvedLanguage: "en", SMS: &domain.SMSPreview{Body: "Ready"},
	}, nil)
	provider := &fakeChannelProvider{result: &domain.ChannelDeliveryResult{Provider: "twilio", ProviderMessageID: "SM123", Status: "queued"}}
	ledger := &fakeChannelSendRepository{created: true, confirmErr: errors.New("database unavailable")}
	service, err := NewChannelMessageService(ChannelMessageServiceConfig{
		Auth: auth, WorkspaceRepo: workspaceRepo, ContactRepo: contactRepo,
		EndpointRepo:    &fakeContactEndpointRepository{endpoints: []*domain.ContactEndpoint{endpoint}},
		TemplateService: templateService, SendRepo: ledger,
		ProviderResolver: fakeChannelProviderResolver{provider: provider}, APIEndpoint: "https://notify.example.com",
	})
	require.NoError(t, err)

	response, err := service.Send(context.Background(), &domain.SendChannelMessageRequest{
		WorkspaceID: "ws-1", EffectKey: "effect-confirm-failure", Channel: domain.ChannelSMS,
		IntegrationID: "twilio-main", ContactEmail: contact.Email, TemplateID: template.ID,
	})
	require.ErrorContains(t, err, "persist confirmed channel send")
	require.NotNil(t, response)
	assert.Equal(t, domain.ChannelSendUnknown, response.Execution.Status)
	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, 1, ledger.confirmed)
	assert.Equal(t, 1, ledger.failed)
}
func (r *fakeChannelSendRepository) Fail(_ context.Context, _, _ string, status domain.ChannelSendStatus, lastError string, _ time.Time) (bool, error) {
	r.failed++
	r.stored.Status = status
	r.stored.LastError = lastError
	return true, nil
}

type fakeContactEndpointRepository struct{ endpoints []*domain.ContactEndpoint }

func (r *fakeContactEndpointRepository) Upsert(context.Context, string, string, *domain.ContactEndpoint) error {
	return nil
}
func (r *fakeContactEndpointRepository) Disable(context.Context, string, string, string) error {
	return nil
}
func (r *fakeContactEndpointRepository) ListActiveByEmail(context.Context, string, string, string) ([]*domain.ContactEndpoint, error) {
	return r.endpoints, nil
}

type fakeChannelProvider struct {
	calls   int
	request domain.ChannelDeliveryRequest
	result  *domain.ChannelDeliveryResult
	err     error
}

func (p *fakeChannelProvider) Send(_ context.Context, request domain.ChannelDeliveryRequest) (*domain.ChannelDeliveryResult, error) {
	p.calls++
	p.request = request
	return p.result, p.err
}

type fakeChannelProviderResolver struct{ provider domain.ChannelProvider }

func (r fakeChannelProviderResolver) Resolve(*domain.Workspace, string, string) (domain.ChannelProvider, error) {
	return r.provider, nil
}

func TestChannelMessageServiceSendRendersAndConfirmsSMS(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := domainmocks.NewMockAuthService(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := domainmocks.NewMockContactRepository(ctrl)
	templateService := domainmocks.NewMockTemplateService(ctrl)
	workspace := &domain.Workspace{ID: "ws-1", Settings: domain.WorkspaceSettings{
		SecretKey: "workspace-secret", DefaultLanguage: "en", WebsiteURL: "https://shop.example.com",
	}}
	contact := &domain.Contact{Email: "user@example.com", FirstName: &domain.NullableString{String: "Alice"}}
	template := &domain.Template{ID: "ready", Version: 3, Channel: domain.ChannelSMS, SMS: &domain.SMSTemplate{Body: "Hello {{ contact.first_name }}"}}
	endpoint := &domain.ContactEndpoint{EndpointID: "phone-1", Email: contact.Email, Channel: domain.ChannelSMS, Provider: domain.EndpointProviderTwilio, Platform: domain.EndpointPlatformPhone, Address: "+15557654321", Locale: "en-US", Enabled: true}

	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").Return(context.Background(), nil, &domain.UserWorkspace{Permissions: domain.UserPermissions{
		domain.PermissionResourceTransactional: {Write: true},
	}}, nil)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(workspace, nil)
	contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws-1", contact.Email).Return(contact, nil)
	templateService.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", "ready", int64(0)).Return(template, nil)
	templateService.EXPECT().PreviewTemplate(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request domain.PreviewTemplateRequest) (*domain.PreviewTemplateResponse, error) {
			assert.Equal(t, "Alice", request.TestData["contact"].(domain.MapOfAny)["first_name"])
			assert.Equal(t, "en", request.Language)
			return &domain.PreviewTemplateResponse{ResolvedLanguage: "en", SMS: &domain.SMSPreview{Body: "Hello Alice"}}, nil
		},
	)
	provider := &fakeChannelProvider{result: &domain.ChannelDeliveryResult{Provider: "twilio", ProviderMessageID: "SM123", Status: "queued"}}
	ledger := &fakeChannelSendRepository{created: true}
	service, err := NewChannelMessageService(ChannelMessageServiceConfig{
		Auth: auth, WorkspaceRepo: workspaceRepo, ContactRepo: contactRepo,
		EndpointRepo:    &fakeContactEndpointRepository{endpoints: []*domain.ContactEndpoint{endpoint}},
		TemplateService: templateService, SendRepo: ledger,
		ProviderResolver: fakeChannelProviderResolver{provider: provider}, APIEndpoint: "https://notify.example.com",
	})
	require.NoError(t, err)

	response, err := service.Send(context.Background(), &domain.SendChannelMessageRequest{
		WorkspaceID: "ws-1", EffectKey: "order-42:sms", Channel: domain.ChannelSMS,
		IntegrationID: "twilio-main", ContactEmail: contact.Email, TemplateID: "ready",
	})
	require.NoError(t, err)
	assert.False(t, response.Duplicate)
	assert.Equal(t, domain.ChannelSendConfirmed, response.Execution.Status)
	assert.Equal(t, "SM123", response.Execution.ProviderMessageID)
	assert.Equal(t, "Hello Alice", provider.request.SMS.Body)
	assert.Equal(t, "+15557654321", provider.request.Recipient)
	assert.Contains(t, provider.request.StatusCallback, "workspace_id=ws-1")
	assert.Contains(t, provider.request.StatusCallback, "integration_id=twilio-main")
	assert.Contains(t, provider.request.StatusCallback, "effect_key=order-42%3Asms")
	assert.Contains(t, provider.request.StatusCallback, "message_id="+response.Execution.MessageID)
	assert.Equal(t, 1, ledger.submitted)
	assert.Equal(t, 1, ledger.confirmed)
}

func TestChannelMessageServiceSendRendersGenericChannelThroughResolvedProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := domainmocks.NewMockAuthService(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := domainmocks.NewMockContactRepository(ctrl)
	templateService := domainmocks.NewMockTemplateService(ctrl)
	workspace := &domain.Workspace{ID: "ws-1", Settings: domain.WorkspaceSettings{SecretKey: "secret", DefaultLanguage: "en"}}
	contact := &domain.Contact{Email: "user@example.com"}
	template := &domain.Template{
		ID: "telegram-ready", Version: 4, Channel: "telegram", ContentSchemaVersion: 1,
		Content: &domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "Hello {{ contact.first_name }}"},
	}
	endpoint := &domain.ContactEndpoint{EndpointID: "telegram-1", Channel: "telegram", Provider: domain.EndpointProviderChannelWebhook, Platform: "telegram_mobile", Address: "chat-123", Locale: "kk-KZ", Enabled: true}

	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").Return(context.Background(), nil, &domain.UserWorkspace{Permissions: domain.UserPermissions{domain.PermissionResourceTransactional: {Write: true}}}, nil)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(workspace, nil)
	contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws-1", contact.Email).Return(contact, nil)
	templateService.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", template.ID, int64(0)).Return(template, nil)
	templateService.EXPECT().PreviewTemplate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request domain.PreviewTemplateRequest) (*domain.PreviewTemplateResponse, error) {
		assert.Equal(t, "telegram", request.Channel)
		assert.Equal(t, "telegram_mobile", request.Profile)
		assert.Same(t, template.Content, request.Content)
		return &domain.PreviewTemplateResponse{
			ResolvedLanguage: "kk",
			ChannelPreview:   &domain.GenericChannelPreview{Profile: "telegram_mobile", Direction: "ltr", Message: domain.RenderedChannelMessage{Family: domain.ContentFamilyText, Body: "Сәлем"}},
		}, nil
	})
	provider := &fakeChannelProvider{result: &domain.ChannelDeliveryResult{Provider: "channel_webhook", ProviderMessageID: "provider-1", Status: "accepted"}}
	ledger := &fakeChannelSendRepository{created: true}
	service, err := NewChannelMessageService(ChannelMessageServiceConfig{
		Auth: auth, WorkspaceRepo: workspaceRepo, ContactRepo: contactRepo,
		EndpointRepo:    &fakeContactEndpointRepository{endpoints: []*domain.ContactEndpoint{endpoint}},
		TemplateService: templateService, SendRepo: ledger,
		ProviderResolver: fakeChannelProviderResolver{provider: provider}, APIEndpoint: "https://notify.example.com",
	})
	require.NoError(t, err)

	response, err := service.Send(context.Background(), &domain.SendChannelMessageRequest{
		WorkspaceID: "ws-1", EffectKey: "effect-telegram", Channel: "telegram",
		IntegrationID: "bridge-main", ContactEmail: contact.Email, TemplateID: template.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.ChannelSendConfirmed, response.Execution.Status)
	require.NotNil(t, provider.request.Generic)
	assert.Equal(t, "Сәлем", provider.request.Generic.Body)
	assert.Equal(t, "telegram_mobile", provider.request.Platform)
	assert.Equal(t, "kk", provider.request.Locale)
	assert.Equal(t, template.ID, provider.request.TemplateID)
	assert.Equal(t, template.Version, provider.request.TemplateVersion)
}

func TestChannelMessageServiceSendDuplicateNeverCallsProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := domainmocks.NewMockAuthService(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := domainmocks.NewMockContactRepository(ctrl)
	templateService := domainmocks.NewMockTemplateService(ctrl)
	workspace := &domain.Workspace{ID: "ws-1", Settings: domain.WorkspaceSettings{SecretKey: "secret", DefaultLanguage: "en"}}
	contact := &domain.Contact{Email: "user@example.com"}
	template := &domain.Template{ID: "push-ready", Version: 2, Channel: domain.ChannelPush, Push: &domain.PushTemplate{Title: "Ready", Body: "Open app"}}
	endpoint := &domain.ContactEndpoint{EndpointID: "device-1", Channel: domain.ChannelPush, Provider: domain.PushProviderFCM, Platform: domain.EndpointPlatformAndroid, Address: "token", Enabled: true}

	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").Return(context.Background(), nil, &domain.UserWorkspace{Permissions: domain.UserPermissions{
		domain.PermissionResourceTransactional: {Write: true},
	}}, nil)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(workspace, nil)
	contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws-1", contact.Email).Return(contact, nil)
	templateService.EXPECT().GetTemplateByID(gomock.Any(), "ws-1", "push-ready", int64(0)).Return(template, nil)
	templateService.EXPECT().PreviewTemplate(gomock.Any(), gomock.Any()).Return(&domain.PreviewTemplateResponse{
		ResolvedLanguage: "en", Push: &domain.PushPreview{Title: "Ready", Body: "Open app", Platform: domain.EndpointPlatformAndroid},
	}, nil)
	provider := &fakeChannelProvider{result: &domain.ChannelDeliveryResult{Provider: "fcm", ProviderMessageID: "projects/x/messages/1"}}
	ledger := &fakeChannelSendRepository{created: false, stored: domain.ChannelSendExecution{
		EffectKey: "effect-1", Status: domain.ChannelSendConfirmed, Provider: "fcm", ProviderMessageID: "projects/x/messages/1",
	}}
	service, err := NewChannelMessageService(ChannelMessageServiceConfig{
		Auth: auth, WorkspaceRepo: workspaceRepo, ContactRepo: contactRepo,
		EndpointRepo:    &fakeContactEndpointRepository{endpoints: []*domain.ContactEndpoint{endpoint}},
		TemplateService: templateService, SendRepo: ledger,
		ProviderResolver: fakeChannelProviderResolver{provider: provider}, APIEndpoint: "https://notify.example.com",
	})
	require.NoError(t, err)

	response, err := service.Send(context.Background(), &domain.SendChannelMessageRequest{
		WorkspaceID: "ws-1", EffectKey: "effect-1", Channel: domain.ChannelPush,
		IntegrationID: "fcm-main", ContactEmail: contact.Email, TemplateID: template.ID,
	})
	require.NoError(t, err)
	assert.True(t, response.Duplicate)
	assert.Equal(t, 0, provider.calls)
	assert.Zero(t, ledger.submitted)
}
