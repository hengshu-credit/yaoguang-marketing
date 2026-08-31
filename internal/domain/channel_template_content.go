package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	ChannelTemplateContentSchemaVersion = 1
	maxChannelFooterRunes               = 1024
	maxChannelActions                   = 10
	maxChannelCards                     = 10
	maxChannelDataBytes                 = 128 * 1024
)

type ChannelMedia struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	AltText string `json:"alt_text,omitempty"`
}

type ChannelAction struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type ChannelCard struct {
	Title   string          `json:"title,omitempty"`
	Body    string          `json:"body,omitempty"`
	Media   *ChannelMedia   `json:"media,omitempty"`
	Actions []ChannelAction `json:"actions,omitempty"`
}

type TemplateParameterBinding struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ExternalTemplateReference struct {
	ID         string                     `json:"id"`
	Language   string                     `json:"language"`
	Category   string                     `json:"category,omitempty"`
	Parameters []TemplateParameterBinding `json:"parameters,omitempty"`
}

type WebhookPayloadTemplate struct {
	ContentType string            `json:"content_type"`
	Body        string            `json:"body"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type ChannelTemplateContent struct {
	Family           ContentFamily              `json:"family"`
	Title            string                     `json:"title,omitempty"`
	Body             string                     `json:"body,omitempty"`
	Footer           string                     `json:"footer,omitempty"`
	Media            *ChannelMedia              `json:"media,omitempty"`
	Actions          []ChannelAction            `json:"actions,omitempty"`
	Cards            []ChannelCard              `json:"cards,omitempty"`
	ExternalTemplate *ExternalTemplateReference `json:"external_template,omitempty"`
	Data             MapOfAny                   `json:"data,omitempty"`
	Webhook          *WebhookPayloadTemplate    `json:"webhook,omitempty"`
}

func (content *ChannelTemplateContent) ValidateForChannel(channel string) error {
	if content == nil {
		return fmt.Errorf("content is required for channel '%s'", channel)
	}
	definition, ok := FindChannelDefinition(channel)
	if !ok {
		return fmt.Errorf("unknown template channel '%s'", channel)
	}
	if !containsContentFamily(definition.ContentFamilies, content.Family) {
		return fmt.Errorf("channel '%s' does not support content family '%s'", definition.ID, content.Family)
	}
	if err := content.validateShape(definition); err != nil {
		return fmt.Errorf("invalid %s template content: %w", definition.ID, err)
	}
	return nil
}

func containsContentFamily(families []ContentFamily, family ContentFamily) bool {
	for _, candidate := range families {
		if candidate == family {
			return true
		}
	}
	return false
}

func (content *ChannelTemplateContent) validateShape(definition ChannelDefinition) error {
	switch content.Family {
	case ContentFamilyText:
		if strings.TrimSpace(content.Body) == "" {
			return fmt.Errorf("body is required")
		}
	case ContentFamilyNotification:
		if strings.TrimSpace(content.Title) == "" {
			return fmt.Errorf("title is required")
		}
		if strings.TrimSpace(content.Body) == "" {
			return fmt.Errorf("body is required")
		}
	case ContentFamilyRichCard, ContentFamilyWorkMessage:
		if strings.TrimSpace(content.Title) == "" && strings.TrimSpace(content.Body) == "" {
			return fmt.Errorf("title or body is required")
		}
	case ContentFamilyCarousel:
		if len(content.Cards) < 2 {
			return fmt.Errorf("carousel requires at least 2 cards")
		}
	case ContentFamilyExternalTemplate:
		if content.ExternalTemplate == nil {
			return fmt.Errorf("external template is required")
		}
		if strings.TrimSpace(content.ExternalTemplate.ID) == "" {
			return fmt.Errorf("external template id is required")
		}
		if strings.TrimSpace(content.ExternalTemplate.Language) == "" {
			return fmt.Errorf("external template language is required")
		}
	case ContentFamilyWebhookPayload:
		if content.Webhook == nil {
			return fmt.Errorf("webhook payload is required")
		}
		if content.Webhook.ContentType != "application/json" {
			return fmt.Errorf("webhook content_type must be application/json")
		}
		if strings.TrimSpace(content.Webhook.Body) == "" {
			return fmt.Errorf("webhook body is required")
		}
	}

	if definition.Limits.MaxTitleRunes > 0 && utf8.RuneCountInString(content.Title) > definition.Limits.MaxTitleRunes {
		return fmt.Errorf("title must not exceed %d characters", definition.Limits.MaxTitleRunes)
	}
	if definition.Limits.MaxBodyRunes > 0 && utf8.RuneCountInString(content.Body) > definition.Limits.MaxBodyRunes {
		return fmt.Errorf("body must not exceed %d characters", definition.Limits.MaxBodyRunes)
	}
	if utf8.RuneCountInString(content.Footer) > maxChannelFooterRunes {
		return fmt.Errorf("footer must not exceed %d characters", maxChannelFooterRunes)
	}
	if len(content.Actions) > maxAllowedActions(definition) {
		return fmt.Errorf("actions must not exceed %d items", maxAllowedActions(definition))
	}
	if len(content.Cards) > maxAllowedCards(definition) {
		return fmt.Errorf("cards must not exceed %d items", maxAllowedCards(definition))
	}
	if err := validateChannelMedia(content.Media); err != nil {
		return err
	}
	for index, action := range content.Actions {
		if err := validateChannelAction(action); err != nil {
			return fmt.Errorf("action %d: %w", index, err)
		}
	}
	for index, card := range content.Cards {
		if strings.TrimSpace(card.Title) == "" && strings.TrimSpace(card.Body) == "" {
			return fmt.Errorf("card %d: title or body is required", index)
		}
		if err := validateChannelMedia(card.Media); err != nil {
			return fmt.Errorf("card %d: %w", index, err)
		}
		for actionIndex, action := range card.Actions {
			if err := validateChannelAction(action); err != nil {
				return fmt.Errorf("card %d action %d: %w", index, actionIndex, err)
			}
		}
	}
	encoded, err := json.Marshal(content.Data)
	if err != nil {
		return fmt.Errorf("data must be valid JSON: %w", err)
	}
	if len(encoded) > maxChannelDataBytes {
		return fmt.Errorf("data must not exceed %d bytes", maxChannelDataBytes)
	}
	return nil
}

func maxAllowedActions(definition ChannelDefinition) int {
	if definition.Limits.MaxActions > 0 && definition.Limits.MaxActions < maxChannelActions {
		return definition.Limits.MaxActions
	}
	return maxChannelActions
}

func maxAllowedCards(definition ChannelDefinition) int {
	if definition.Limits.MaxCards > 0 && definition.Limits.MaxCards < maxChannelCards {
		return definition.Limits.MaxCards
	}
	return maxChannelCards
}

func validateChannelMedia(media *ChannelMedia) error {
	if media == nil {
		return nil
	}
	if media.Type != "image" && media.Type != "video" && media.Type != "audio" && media.Type != "file" {
		return fmt.Errorf("media type must be image, video, audio, or file")
	}
	if !strings.HasPrefix(media.URL, "https://") {
		return fmt.Errorf("media url must be an absolute https URL")
	}
	if !containsLiquidMarkup(media.URL) {
		parsed, err := url.ParseRequestURI(media.URL)
		if err != nil || parsed.Host == "" {
			return fmt.Errorf("media url must be an absolute https URL")
		}
	}
	return nil
}

func validateChannelAction(action ChannelAction) error {
	if strings.TrimSpace(action.Label) == "" {
		return fmt.Errorf("action label is required")
	}
	switch action.Type {
	case "url", "deep_link", "reply", "phone":
	default:
		return fmt.Errorf("action type must be url, deep_link, reply, or phone")
	}
	if strings.TrimSpace(action.Value) == "" {
		return fmt.Errorf("action value is required")
	}
	return nil
}
