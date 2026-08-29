package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

type ChannelProviderResolver interface {
	Resolve(*domain.Workspace, string, string) (domain.ChannelProvider, error)
}

type WorkspaceChannelProviderResolver struct{ client *http.Client }

func NewWorkspaceChannelProviderResolver(client *http.Client) *WorkspaceChannelProviderResolver {
	return &WorkspaceChannelProviderResolver{client: client}
}

func (r *WorkspaceChannelProviderResolver) Resolve(workspace *domain.Workspace, integrationID, channel string) (domain.ChannelProvider, error) {
	if workspace == nil {
		return nil, errors.New("workspace is required")
	}
	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil {
		return nil, fmt.Errorf("integration %s not found", integrationID)
	}
	switch channel {
	case domain.ChannelSMS:
		if integration.Type != domain.IntegrationTypeSMS || integration.SMSProvider == nil || integration.SMSProvider.Twilio == nil {
			return nil, fmt.Errorf("integration %s is not a Twilio SMS provider", integrationID)
		}
		return NewTwilioChannelProvider(*integration.SMSProvider.Twilio, r.client, "")
	case domain.ChannelPush:
		if integration.Type != domain.IntegrationTypePush || integration.PushProvider == nil || integration.PushProvider.FCM == nil {
			return nil, fmt.Errorf("integration %s is not an FCM push provider", integrationID)
		}
		return NewFCMChannelProvider(*integration.PushProvider.FCM, r.client, "", nil)
	default:
		return nil, fmt.Errorf("unsupported channel %s", channel)
	}
}

type ChannelMessageServiceConfig struct {
	Auth             domain.AuthService
	WorkspaceRepo    domain.WorkspaceRepository
	ContactRepo      domain.ContactRepository
	EndpointRepo     domain.ContactEndpointRepository
	TemplateService  domain.TemplateService
	SendRepo         domain.ChannelSendRepository
	ProviderResolver ChannelProviderResolver
	APIEndpoint      string
}

type ChannelMessageService struct {
	auth             domain.AuthService
	workspaceRepo    domain.WorkspaceRepository
	contactRepo      domain.ContactRepository
	endpointRepo     domain.ContactEndpointRepository
	templateService  domain.TemplateService
	sendRepo         domain.ChannelSendRepository
	providerResolver ChannelProviderResolver
	apiEndpoint      string
}

func NewChannelMessageService(config ChannelMessageServiceConfig) (*ChannelMessageService, error) {
	if config.Auth == nil || config.WorkspaceRepo == nil || config.ContactRepo == nil ||
		config.EndpointRepo == nil || config.TemplateService == nil || config.SendRepo == nil ||
		config.ProviderResolver == nil {
		return nil, errors.New("channel message service dependencies are required")
	}
	return &ChannelMessageService{
		auth: config.Auth, workspaceRepo: config.WorkspaceRepo, contactRepo: config.ContactRepo,
		endpointRepo: config.EndpointRepo, templateService: config.TemplateService,
		sendRepo: config.SendRepo, providerResolver: config.ProviderResolver,
		apiEndpoint: strings.TrimRight(config.APIEndpoint, "/"),
	}, nil
}

func (s *ChannelMessageService) Send(ctx context.Context, request *domain.SendChannelMessageRequest) (*domain.SendChannelMessageResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authenticatedCtx := ctx
	if ctx.Value(domain.SystemCallKey) == nil {
		var userWorkspace *domain.UserWorkspace
		var err error
		authenticatedCtx, _, userWorkspace, err = s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("authenticate channel send: %w", err)
		}
		if !userWorkspace.HasPermission(domain.PermissionResourceTransactional, domain.PermissionTypeWrite) {
			return nil, domain.NewPermissionError(domain.PermissionResourceTransactional, domain.PermissionTypeWrite,
				"Insufficient permissions: write access to transactional notifications required")
		}
	}
	workspace, err := s.workspaceRepo.GetByID(authenticatedCtx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load channel send workspace: %w", err)
	}
	if workspace == nil {
		return nil, fmt.Errorf("load channel send workspace: workspace not found")
	}
	contact, err := s.contactRepo.GetContactByEmail(authenticatedCtx, request.WorkspaceID, request.ContactEmail)
	if err != nil {
		return nil, fmt.Errorf("load channel send contact: %w", err)
	}
	template, err := s.templateService.GetTemplateByID(systemContext(authenticatedCtx), request.WorkspaceID, request.TemplateID, request.TemplateVersion)
	if err != nil {
		return nil, fmt.Errorf("load channel send template: %w", err)
	}
	if template.Channel != request.Channel {
		return nil, domain.NewValidationError(fmt.Sprintf("template %s is for channel %s, not %s", template.ID, template.Channel, request.Channel))
	}
	endpoints, err := s.endpointRepo.ListActiveByEmail(authenticatedCtx, request.WorkspaceID, request.ContactEmail, request.Channel)
	if err != nil {
		return nil, fmt.Errorf("load channel send endpoints: %w", err)
	}
	endpoint := selectChannelEndpoint(endpoints, request.EndpointID)
	if endpoint == nil {
		return nil, domain.NewValidationError("no active matching contact endpoint")
	}
	provider, err := s.providerResolver.Resolve(workspace, request.IntegrationID, request.Channel)
	if err != nil {
		return nil, domain.NewValidationError(err.Error())
	}

	request.EndpointID = endpoint.EndpointID
	request.TemplateVersion = template.Version
	if request.Language == "" {
		request.Language = resolvableChannelLanguage(endpoint.Locale)
		if request.Language == "" && contact.Language != nil && !contact.Language.IsNull {
			request.Language = resolvableChannelLanguage(contact.Language.String)
		}
	}
	messageID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(request.WorkspaceID+":"+request.EffectKey+":"+endpoint.EndpointID)).String()
	trackingEndpoint := workspace.Settings.ResolveEndpoint(s.apiEndpoint)
	templateData, err := domain.BuildTemplateData(domain.TemplateDataRequest{
		WorkspaceID: request.WorkspaceID, WorkspaceSecretKey: workspace.Settings.SecretKey,
		WorkspaceWebsiteURL: workspace.Settings.WebsiteURL,
		ContactWithList:     domain.ContactWithList{Contact: contact}, MessageID: messageID,
		ProvidedData: request.Data, TrackingSettings: notifuse_mjml.TrackingSettings{Endpoint: trackingEndpoint},
	})
	if err != nil {
		return nil, fmt.Errorf("build channel template data: %w", err)
	}
	preview, err := s.templateService.PreviewTemplate(systemContext(authenticatedCtx), domain.PreviewTemplateRequest{
		WorkspaceID: request.WorkspaceID, Channel: request.Channel, SMS: template.SMS, Push: template.Push,
		Translations: template.Translations, Language: request.Language, Platform: endpoint.Platform, TestData: templateData,
	})
	if err != nil {
		return nil, fmt.Errorf("render channel template: %w", err)
	}
	delivery := domain.ChannelDeliveryRequest{
		Channel: request.Channel, Recipient: endpoint.Address, EffectKey: request.EffectKey,
	}
	if preview.SMS != nil {
		delivery.SMS = &domain.SMSTemplate{Body: preview.SMS.Body, SenderID: preview.SMS.SenderID}
		callback, _ := url.Parse(trackingEndpoint + "/webhooks/delivery/twilio")
		query := callback.Query()
		query.Set("workspace_id", request.WorkspaceID)
		query.Set("integration_id", request.IntegrationID)
		query.Set("message_id", messageID)
		query.Set("effect_key", request.EffectKey)
		callback.RawQuery = query.Encode()
		delivery.StatusCallback = callback.String()
	}
	if preview.Push != nil {
		delivery.Push = &domain.PushTemplate{Title: preview.Push.Title, Body: preview.Push.Body,
			ImageURL: preview.Push.ImageURL, DeepLink: preview.Push.DeepLink, Data: preview.Push.Data}
	}
	hash, err := request.RequestHash()
	if err != nil {
		return nil, err
	}
	execution := domain.ChannelSendExecution{
		EffectKey: request.EffectKey, RequestHash: hash, MessageID: messageID,
		Channel: request.Channel, IntegrationID: request.IntegrationID, ContactEmail: request.ContactEmail,
		EndpointID: endpoint.EndpointID, TemplateID: template.ID, TemplateVersion: template.Version,
		Language: preview.ResolvedLanguage, Status: domain.ChannelSendReserved,
	}
	stored, created, err := s.sendRepo.Reserve(authenticatedCtx, request.WorkspaceID, execution)
	if err != nil {
		if errors.Is(err, domain.ErrChannelSendHashConflict) {
			return nil, domain.NewValidationError(err.Error())
		}
		return nil, err
	}
	if !created {
		return &domain.SendChannelMessageResponse{Execution: stored, Duplicate: true}, nil
	}
	now := time.Now().UTC()
	transitioned, err := s.sendRepo.MarkSubmitted(authenticatedCtx, request.WorkspaceID, request.EffectKey, now)
	if err != nil {
		return nil, fmt.Errorf("submit channel send execution: %w", err)
	}
	if !transitioned {
		return nil, fmt.Errorf("submit channel send execution: state transition rejected")
	}
	execution.Status = domain.ChannelSendSubmitted
	execution.Attempts = 1
	result, sendErr := provider.Send(authenticatedCtx, delivery)
	if sendErr != nil {
		status := domain.ChannelSendFailed
		if errors.Is(sendErr, ErrSideEffectOutcomeUnknown) {
			status = domain.ChannelSendUnknown
		}
		_, persistErr := s.sendRepo.Fail(authenticatedCtx, request.WorkspaceID, request.EffectKey, status, sendErr.Error(), time.Now().UTC())
		execution.Status, execution.LastError = status, sendErr.Error()
		if persistErr != nil {
			return &domain.SendChannelMessageResponse{Execution: execution}, errors.Join(sendErr, persistErr)
		}
		return &domain.SendChannelMessageResponse{Execution: execution}, sendErr
	}
	sentAt := time.Now().UTC()
	externalID := result.ProviderMessageID
	statusInfo := result.Status
	metadata := make(map[string]interface{}, len(request.Metadata)+4)
	for key, value := range request.Metadata {
		metadata[key] = value
	}
	metadata["effect_key"] = request.EffectKey
	metadata["endpoint_id"] = endpoint.EndpointID
	metadata["integration_id"] = request.IntegrationID
	metadata["provider"] = result.Provider
	message := &domain.MessageHistory{
		ID: messageID, ExternalID: &externalID, ContactEmail: request.ContactEmail,
		TemplateID: template.ID, TemplateVersion: template.Version, Channel: request.Channel,
		StatusInfo: &statusInfo, SentAt: sentAt,
		MessageData: domain.MessageData{Data: templateData, Metadata: metadata},
	}
	execution.Provider = result.Provider
	execution.ProviderMessageID = result.ProviderMessageID
	markConfirmationUnknown := func(cause error) error {
		lastError := cause.Error()
		failedAt := time.Now().UTC()
		transitioned, persistErr := s.sendRepo.Fail(
			authenticatedCtx,
			request.WorkspaceID,
			request.EffectKey,
			domain.ChannelSendUnknown,
			lastError,
			failedAt,
		)
		execution.Status = domain.ChannelSendUnknown
		execution.LastError = lastError
		execution.UpdatedAt = failedAt
		if persistErr != nil {
			return errors.Join(cause, fmt.Errorf("persist unknown channel send: %w", persistErr))
		}
		if !transitioned {
			return errors.Join(cause, errors.New("persist unknown channel send: state transition rejected"))
		}
		return cause
	}
	confirmed, err := s.sendRepo.Confirm(authenticatedCtx, request.WorkspaceID, request.EffectKey, result.Provider, result.ProviderMessageID, workspace.Settings.SecretKey, message, sentAt)
	if err != nil {
		confirmErr := fmt.Errorf("persist confirmed channel send: %w", err)
		return &domain.SendChannelMessageResponse{Execution: execution}, markConfirmationUnknown(confirmErr)
	}
	if !confirmed {
		confirmErr := errors.New("persist confirmed channel send: state transition rejected")
		return &domain.SendChannelMessageResponse{Execution: execution}, markConfirmationUnknown(confirmErr)
	}
	execution.Status = domain.ChannelSendConfirmed
	execution.UpdatedAt = sentAt
	return &domain.SendChannelMessageResponse{Execution: execution}, nil
}

func resolvableChannelLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" || domain.IsValidLanguage(language) {
		return language
	}
	if separator := strings.IndexByte(language, '-'); separator > 0 {
		base := strings.ToLower(language[:separator])
		if domain.IsValidLanguage(base) {
			return base
		}
	}
	return ""
}

func systemContext(ctx context.Context) context.Context {
	if ctx.Value(domain.SystemCallKey) != nil {
		return ctx
	}
	return context.WithValue(ctx, domain.SystemCallKey, true)
}

func selectChannelEndpoint(endpoints []*domain.ContactEndpoint, endpointID string) *domain.ContactEndpoint {
	for _, endpoint := range endpoints {
		if endpoint == nil || !endpoint.Enabled {
			continue
		}
		if endpointID == "" || endpoint.EndpointID == endpointID {
			return endpoint
		}
	}
	return nil
}
