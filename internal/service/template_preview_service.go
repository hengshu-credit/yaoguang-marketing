package service

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/notifuse_mjml"
)

const maxPushPreviewDepth = 16

func (s *TemplateService) PreviewTemplate(ctx context.Context, request domain.PreviewTemplateRequest) (*domain.PreviewTemplateResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}

	authenticatedCtx := ctx
	if ctx.Value(domain.SystemCallKey) == nil {
		var userWorkspace *domain.UserWorkspace
		var err error
		authenticatedCtx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate user: %w", err)
		}
		if !userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(
				domain.PermissionResourceTemplates,
				domain.PermissionTypeRead,
				"Insufficient permissions: read access to templates required",
			)
		}
	}

	workspace, err := s.workspaceRepo.GetByID(authenticatedCtx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace for template preview: %w", err)
	}
	if workspace == nil {
		return nil, fmt.Errorf("workspace not found: %s", request.WorkspaceID)
	}
	if err := validatePreviewTranslationLanguages(workspace, request.Translations); err != nil {
		return nil, err
	}

	defaultLanguage := workspace.Settings.DefaultLanguage
	if defaultLanguage == "" {
		defaultLanguage = domain.DefaultLanguageCode
	}
	requestedLanguage := request.Language
	if requestedLanguage == "" {
		requestedLanguage = defaultLanguage
	}
	resolvedLanguage := defaultLanguage
	fallbackUsed := requestedLanguage != defaultLanguage
	if translation, ok := request.Translations[requestedLanguage]; ok {
		if (request.Channel == domain.ChannelSMS && translation.SMS != nil) ||
			(request.Channel == domain.ChannelPush && translation.Push != nil) {
			resolvedLanguage = requestedLanguage
			fallbackUsed = false
		}
	}

	templateData := clonePreviewData(request.TestData)
	templateData["workspace"] = domain.BuildWorkspaceTemplateVars(
		workspace.Settings.ResolveEndpoint(s.apiEndpoint),
		workspace.Settings.WebsiteURL,
	)

	response := &domain.PreviewTemplateResponse{
		Channel:           request.Channel,
		RequestedLanguage: request.Language,
		ResolvedLanguage:  resolvedLanguage,
		FallbackUsed:      fallbackUsed,
		TestData:          templateData,
	}
	template := &domain.Template{
		SMS: request.SMS, Push: request.Push, Translations: request.Translations,
	}

	switch request.Channel {
	case domain.ChannelSMS:
		content := template.ResolveSMSContent(requestedLanguage, defaultLanguage)
		preview, err := renderSMSPreview(content, templateData)
		if err != nil {
			return nil, err
		}
		response.SMS = preview
	case domain.ChannelPush:
		content := template.ResolvePushContent(requestedLanguage, defaultLanguage)
		preview, err := renderPushPreview(content, request.Platform, templateData)
		if err != nil {
			return nil, err
		}
		response.Push = preview
	}
	return response, nil
}

func validatePreviewTranslationLanguages(workspace *domain.Workspace, translations map[string]domain.TemplateTranslation) error {
	if len(translations) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(workspace.Settings.Languages)+1)
	for _, language := range workspace.Settings.Languages {
		allowed[language] = struct{}{}
	}
	if len(allowed) == 0 && workspace.Settings.DefaultLanguage != "" {
		allowed[workspace.Settings.DefaultLanguage] = struct{}{}
	}
	for language := range translations {
		if _, ok := allowed[language]; !ok {
			return fmt.Errorf("translation language '%s' is not in workspace's configured languages", language)
		}
	}
	return nil
}

func clonePreviewData(source domain.MapOfAny) domain.MapOfAny {
	clone := make(domain.MapOfAny, len(source)+1)
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func renderPreviewString(content string, data domain.MapOfAny, contextLabel string) (string, error) {
	rendered, err := notifuse_mjml.ProcessLiquidTemplate(content, map[string]interface{}(data), contextLabel)
	if err != nil {
		return "", fmt.Errorf("failed to render %s: %w", contextLabel, err)
	}
	return rendered, nil
}

func renderSMSPreview(content *domain.SMSTemplate, data domain.MapOfAny) (*domain.SMSPreview, error) {
	body, err := renderPreviewString(content.Body, data, "sms_body")
	if err != nil {
		return nil, err
	}
	senderID, err := renderPreviewString(content.SenderID, data, "sms_sender_id")
	if err != nil {
		return nil, err
	}
	rendered := &domain.SMSTemplate{Body: body, SenderID: senderID}
	if err := rendered.Validate(data); err != nil {
		return nil, fmt.Errorf("rendered sms template is invalid: %w", err)
	}
	encoding, units, segments, perSegment, remaining := smsSegments(body)
	return &domain.SMSPreview{
		Body: body, SenderID: senderID, Encoding: encoding,
		CharacterCount: utf8.RuneCountInString(body), UnitCount: units,
		SegmentCount: segments, PerSegment: perSegment, Remaining: remaining,
	}, nil
}

func renderPushPreview(content *domain.PushTemplate, platform string, data domain.MapOfAny) (*domain.PushPreview, error) {
	title, err := renderPreviewString(content.Title, data, "push_title")
	if err != nil {
		return nil, err
	}
	body, err := renderPreviewString(content.Body, data, "push_body")
	if err != nil {
		return nil, err
	}
	imageURL, err := renderPreviewString(content.ImageURL, data, "push_image_url")
	if err != nil {
		return nil, err
	}
	deepLink, err := renderPreviewString(content.DeepLink, data, "push_deep_link")
	if err != nil {
		return nil, err
	}
	renderedDataValue, err := renderPushPreviewValue(content.Data, data, 0)
	if err != nil {
		return nil, err
	}
	renderedData, _ := renderedDataValue.(domain.MapOfAny)
	rendered := &domain.PushTemplate{
		Title: title, Body: body, ImageURL: imageURL, DeepLink: deepLink, Data: renderedData,
	}
	if err := rendered.Validate(data); err != nil {
		return nil, fmt.Errorf("rendered push template is invalid: %w", err)
	}
	payload, err := json.Marshal(domain.MapOfAny{
		"notification": domain.MapOfAny{"title": title, "body": body, "image_url": imageURL},
		"deep_link":    deepLink,
		"data":         renderedData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode push preview payload: %w", err)
	}
	warnings := pushPreviewWarnings(platform, title, body, len(payload))
	return &domain.PushPreview{
		Title: title, Body: body, ImageURL: imageURL, DeepLink: deepLink,
		Data: renderedData, Platform: platform, PayloadBytes: len(payload), Warnings: warnings,
	}, nil
}

func renderPushPreviewValue(value interface{}, data domain.MapOfAny, depth int) (interface{}, error) {
	if depth > maxPushPreviewDepth {
		return nil, fmt.Errorf("push data nesting exceeds %d levels", maxPushPreviewDepth)
	}
	switch typed := value.(type) {
	case string:
		return renderPreviewString(typed, data, "push_data")
	case domain.MapOfAny:
		result := make(domain.MapOfAny, len(typed))
		for key, child := range typed {
			rendered, err := renderPushPreviewValue(child, data, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = rendered
		}
		return result, nil
	case map[string]interface{}:
		result := make(domain.MapOfAny, len(typed))
		for key, child := range typed {
			rendered, err := renderPushPreviewValue(child, data, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = rendered
		}
		return result, nil
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, child := range typed {
			rendered, err := renderPushPreviewValue(child, data, depth+1)
			if err != nil {
				return nil, err
			}
			result[index] = rendered
		}
		return result, nil
	default:
		return value, nil
	}
}

func pushPreviewWarnings(platform, title, body string, payloadBytes int) []domain.PreviewWarning {
	titleLimit, bodyLimit := 65, 240
	switch platform {
	case domain.EndpointPlatformIOS:
		titleLimit, bodyLimit = 50, 178
	case domain.EndpointPlatformWeb:
		titleLimit, bodyLimit = 60, 240
	}
	warnings := make([]domain.PreviewWarning, 0, 3)
	if utf8.RuneCountInString(title) > titleLimit {
		warnings = append(warnings, domain.PreviewWarning{Code: "title_may_truncate", Message: fmt.Sprintf("%s clients may truncate titles over %d characters", platform, titleLimit)})
	}
	if utf8.RuneCountInString(body) > bodyLimit {
		warnings = append(warnings, domain.PreviewWarning{Code: "body_may_truncate", Message: fmt.Sprintf("%s clients may truncate bodies over %d characters", platform, bodyLimit)})
	}
	if payloadBytes > 4096 {
		warnings = append(warnings, domain.PreviewWarning{Code: "payload_exceeds_4kb", Message: "normalized push payload exceeds 4096 bytes"})
	}
	return warnings
}

func smsSegments(body string) (encoding string, units int, segments int, perSegment int, remaining int) {
	if gsmUnits, ok := gsm7Units(body); ok {
		encoding, units = "gsm-7", gsmUnits
		if units <= 160 {
			return encoding, units, 1, 160, 160 - units
		}
		segments = (units + 152) / 153
		return encoding, units, segments, 153, segments*153 - units
	}
	units = len(utf16.Encode([]rune(body)))
	encoding = "ucs-2"
	if units <= 70 {
		return encoding, units, 1, 70, 70 - units
	}
	segments = (units + 66) / 67
	return encoding, units, segments, 67, segments*67 - units
}

var gsm7Basic = func() map[rune]struct{} {
	characters := "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ ÆæßÉ!\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"
	result := make(map[rune]struct{}, len(characters))
	for _, character := range characters {
		result[character] = struct{}{}
	}
	return result
}()

var gsm7Extended = func() map[rune]struct{} {
	result := make(map[rune]struct{})
	for _, character := range "\f^{}\\[~]|€" {
		result[character] = struct{}{}
	}
	return result
}()

func gsm7Units(body string) (int, bool) {
	units := 0
	for _, character := range body {
		if _, ok := gsm7Basic[character]; ok {
			units++
			continue
		}
		if _, ok := gsm7Extended[character]; ok {
			units += 2
			continue
		}
		return 0, false
	}
	return units, true
}
