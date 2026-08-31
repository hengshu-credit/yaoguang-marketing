package service

import (
	"encoding/json"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/notifuse_mjml"
)

const maxGenericPreviewDepth = 16

func renderGenericChannelPreview(
	content *domain.ChannelTemplateContent,
	channel string,
	profile string,
	direction string,
	data domain.MapOfAny,
) (*domain.GenericChannelPreview, error) {
	definition, ok := domain.FindChannelDefinition(channel)
	if !ok {
		return nil, fmt.Errorf("unknown template channel '%s'", channel)
	}
	if !channelDefinitionSupportsProfile(definition, profile) {
		return nil, fmt.Errorf("preview profile '%s' is not supported by channel '%s'", profile, channel)
	}
	if err := content.ValidateForChannel(channel); err != nil {
		return nil, err
	}

	rendered, err := renderChannelTemplateContent(content, data)
	if err != nil {
		return nil, err
	}
	if err := rendered.ValidateForChannel(channel); err != nil {
		return nil, fmt.Errorf("rendered channel content is invalid: %w", err)
	}
	if rendered.Webhook != nil && !json.Valid([]byte(rendered.Webhook.Body)) {
		return nil, fmt.Errorf("rendered webhook body must be valid JSON")
	}
	payload, err := json.Marshal(rendered)
	if err != nil {
		return nil, fmt.Errorf("encode rendered channel preview: %w", err)
	}
	warnings := make([]domain.PreviewWarning, 0)
	if definition.Limits.MaxPayload > 0 && len(payload) > definition.Limits.MaxPayload {
		warnings = append(warnings, domain.PreviewWarning{
			Code:    "payload_limit_exceeded",
			Message: fmt.Sprintf("rendered payload exceeds %d bytes", definition.Limits.MaxPayload),
		})
	}
	return &domain.GenericChannelPreview{
		Profile: profile, Direction: direction, PayloadBytes: len(payload),
		Message: *rendered, Warnings: warnings,
	}, nil
}

func channelDefinitionSupportsProfile(definition domain.ChannelDefinition, profileID string) bool {
	for _, profile := range definition.PreviewProfiles {
		if profile.ID == profileID {
			return true
		}
	}
	return false
}

func renderChannelTemplateContent(content *domain.ChannelTemplateContent, data domain.MapOfAny) (*domain.ChannelTemplateContent, error) {
	rendered := *content
	var err error
	if rendered.Title, err = renderChannelPreviewString(content.Title, data, "channel_title"); err != nil {
		return nil, err
	}
	if rendered.Body, err = renderChannelPreviewString(content.Body, data, "channel_body"); err != nil {
		return nil, err
	}
	if rendered.Footer, err = renderChannelPreviewString(content.Footer, data, "channel_footer"); err != nil {
		return nil, err
	}
	if rendered.Media, err = renderChannelMedia(content.Media, data); err != nil {
		return nil, err
	}
	rendered.Actions = make([]domain.ChannelAction, len(content.Actions))
	for index, action := range content.Actions {
		rendered.Actions[index], err = renderChannelAction(action, data)
		if err != nil {
			return nil, fmt.Errorf("render action %d: %w", index, err)
		}
	}
	rendered.Cards = make([]domain.ChannelCard, len(content.Cards))
	for index, card := range content.Cards {
		rendered.Cards[index], err = renderChannelCard(card, data)
		if err != nil {
			return nil, fmt.Errorf("render card %d: %w", index, err)
		}
	}
	if content.ExternalTemplate != nil {
		external := *content.ExternalTemplate
		if external.ID, err = renderChannelPreviewString(external.ID, data, "external_template_id"); err != nil {
			return nil, err
		}
		if external.Language, err = renderChannelPreviewString(external.Language, data, "external_template_language"); err != nil {
			return nil, err
		}
		if external.Category, err = renderChannelPreviewString(external.Category, data, "external_template_category"); err != nil {
			return nil, err
		}
		external.Parameters = make([]domain.TemplateParameterBinding, len(content.ExternalTemplate.Parameters))
		for index, parameter := range content.ExternalTemplate.Parameters {
			external.Parameters[index].Name = parameter.Name
			external.Parameters[index].Value, err = renderChannelPreviewString(parameter.Value, data, "external_template_parameter")
			if err != nil {
				return nil, err
			}
		}
		rendered.ExternalTemplate = &external
	}
	if content.Webhook != nil {
		webhook := *content.Webhook
		if webhook.Body, err = renderChannelPreviewString(webhook.Body, data, "webhook_body"); err != nil {
			return nil, err
		}
		webhook.Headers = make(map[string]string, len(content.Webhook.Headers))
		for key, value := range content.Webhook.Headers {
			webhook.Headers[key], err = renderChannelPreviewString(value, data, "webhook_header")
			if err != nil {
				return nil, err
			}
		}
		rendered.Webhook = &webhook
	}
	renderedData, err := renderChannelPreviewValue(content.Data, data, 0)
	if err != nil {
		return nil, err
	}
	if renderedData != nil {
		rendered.Data, _ = renderedData.(domain.MapOfAny)
	}
	return &rendered, nil
}

func renderChannelMedia(media *domain.ChannelMedia, data domain.MapOfAny) (*domain.ChannelMedia, error) {
	if media == nil {
		return nil, nil
	}
	rendered := *media
	var err error
	if rendered.URL, err = renderChannelPreviewString(media.URL, data, "channel_media_url"); err != nil {
		return nil, err
	}
	if rendered.AltText, err = renderChannelPreviewString(media.AltText, data, "channel_media_alt_text"); err != nil {
		return nil, err
	}
	return &rendered, nil
}

func renderChannelAction(action domain.ChannelAction, data domain.MapOfAny) (domain.ChannelAction, error) {
	rendered := action
	var err error
	if rendered.Label, err = renderChannelPreviewString(action.Label, data, "channel_action_label"); err != nil {
		return domain.ChannelAction{}, err
	}
	if rendered.Value, err = renderChannelPreviewString(action.Value, data, "channel_action_value"); err != nil {
		return domain.ChannelAction{}, err
	}
	return rendered, nil
}

func renderChannelCard(card domain.ChannelCard, data domain.MapOfAny) (domain.ChannelCard, error) {
	rendered := card
	var err error
	if rendered.Title, err = renderChannelPreviewString(card.Title, data, "channel_card_title"); err != nil {
		return domain.ChannelCard{}, err
	}
	if rendered.Body, err = renderChannelPreviewString(card.Body, data, "channel_card_body"); err != nil {
		return domain.ChannelCard{}, err
	}
	if rendered.Media, err = renderChannelMedia(card.Media, data); err != nil {
		return domain.ChannelCard{}, err
	}
	rendered.Actions = make([]domain.ChannelAction, len(card.Actions))
	for index, action := range card.Actions {
		rendered.Actions[index], err = renderChannelAction(action, data)
		if err != nil {
			return domain.ChannelCard{}, err
		}
	}
	return rendered, nil
}

func renderChannelPreviewValue(value interface{}, data domain.MapOfAny, depth int) (interface{}, error) {
	if depth > maxGenericPreviewDepth {
		return nil, fmt.Errorf("channel data nesting exceeds %d levels", maxGenericPreviewDepth)
	}
	switch typed := value.(type) {
	case string:
		return renderChannelPreviewString(typed, data, "channel_data")
	case domain.MapOfAny:
		result := make(domain.MapOfAny, len(typed))
		for key, child := range typed {
			rendered, err := renderChannelPreviewValue(child, data, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = rendered
		}
		return result, nil
	case map[string]interface{}:
		result := make(domain.MapOfAny, len(typed))
		for key, child := range typed {
			rendered, err := renderChannelPreviewValue(child, data, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = rendered
		}
		return result, nil
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, child := range typed {
			rendered, err := renderChannelPreviewValue(child, data, depth+1)
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

func renderChannelPreviewString(value string, data domain.MapOfAny, label string) (string, error) {
	rendered, err := notifuse_mjml.ProcessLiquidTemplate(value, map[string]interface{}(data), label)
	if err != nil {
		return "", fmt.Errorf("failed to render %s: %w", label, err)
	}
	return rendered, nil
}
