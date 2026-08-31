package domain

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	ChannelSMS = "sms"

	maxSMSBodyRunes     = 10000
	maxPushTitleRunes   = 512
	maxPushBodyRunes    = 4096
	maxPushDataBytes    = 32 * 1024
	maxPreviewDataBytes = 128 * 1024
)

type SMSTemplate struct {
	Body     string `json:"body"`
	SenderID string `json:"sender_id,omitempty"`
}

func (s *SMSTemplate) Validate(_ MapOfAny) error {
	if s == nil || strings.TrimSpace(s.Body) == "" {
		return fmt.Errorf("invalid sms template: body is required")
	}
	if utf8.RuneCountInString(s.Body) > maxSMSBodyRunes {
		return fmt.Errorf("invalid sms template: body must not exceed %d characters", maxSMSBodyRunes)
	}
	if utf8.RuneCountInString(s.SenderID) > 32 {
		return fmt.Errorf("invalid sms template: sender_id must not exceed 32 characters")
	}
	return nil
}

func (s *SMSTemplate) Scan(value interface{}) error { return scanJSONValue(value, s) }
func (s SMSTemplate) Value() (driver.Value, error)  { return json.Marshal(s) }

type PushTemplate struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	ImageURL string   `json:"image_url,omitempty"`
	DeepLink string   `json:"deep_link,omitempty"`
	Data     MapOfAny `json:"data,omitempty"`
}

func (p *PushTemplate) Validate(_ MapOfAny) error {
	if p == nil || strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("invalid push template: title is required")
	}
	if strings.TrimSpace(p.Body) == "" {
		return fmt.Errorf("invalid push template: body is required")
	}
	if utf8.RuneCountInString(p.Title) > maxPushTitleRunes {
		return fmt.Errorf("invalid push template: title must not exceed %d characters", maxPushTitleRunes)
	}
	if utf8.RuneCountInString(p.Body) > maxPushBodyRunes {
		return fmt.Errorf("invalid push template: body must not exceed %d characters", maxPushBodyRunes)
	}
	if p.ImageURL != "" && !containsLiquidMarkup(p.ImageURL) {
		parsed, err := url.ParseRequestURI(p.ImageURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("invalid push template: image_url must be an absolute http or https URL")
		}
	}
	if p.DeepLink != "" && !containsLiquidMarkup(p.DeepLink) {
		parsed, err := url.ParseRequestURI(p.DeepLink)
		if err != nil || parsed.Scheme == "" || strings.ContainsAny(p.DeepLink, "\r\n\t ") {
			return fmt.Errorf("invalid push template: deep_link must be an absolute URL")
		}
	}
	encoded, err := json.Marshal(p.Data)
	if err != nil {
		return fmt.Errorf("invalid push template: data must be valid JSON: %w", err)
	}
	if len(encoded) > maxPushDataBytes {
		return fmt.Errorf("invalid push template: data must not exceed %d bytes", maxPushDataBytes)
	}
	return nil
}

func containsLiquidMarkup(value string) bool {
	return strings.Contains(value, "{{") || strings.Contains(value, "{%")
}

func (p *PushTemplate) Scan(value interface{}) error { return scanJSONValue(value, p) }
func (p PushTemplate) Value() (driver.Value, error)  { return json.Marshal(p) }

func scanJSONValue(value interface{}, destination interface{}) error {
	var data []byte
	switch typed := value.(type) {
	case []byte:
		data = bytes.Clone(typed)
	case string:
		data = []byte(typed)
	case nil:
		return nil
	default:
		return fmt.Errorf("cannot scan %T as JSON", value)
	}
	return json.Unmarshal(data, destination)
}

func isTemplateChannel(channel string) bool {
	return IsRegisteredChannel(channel)
}

func isLegacyTemplateChannel(channel string) bool {
	return channel == ChannelEmail || channel == ChannelWeb || channel == ChannelSMS || channel == ChannelPush
}

func validateTemplateContent(
	channel string,
	email *EmailTemplate,
	web *WebTemplate,
	sms *SMSTemplate,
	push *PushTemplate,
	content *ChannelTemplateContent,
	contentSchemaVersion int,
	testData MapOfAny,
) error {
	if !IsRegisteredChannel(channel) {
		return fmt.Errorf("unknown template channel '%s'", channel)
	}
	contents := []struct {
		name    string
		present bool
	}{
		{name: ChannelEmail, present: email != nil},
		{name: ChannelWeb, present: web != nil},
		{name: ChannelSMS, present: sms != nil},
		{name: ChannelPush, present: push != nil},
	}
	if !isLegacyTemplateChannel(channel) {
		if content == nil {
			return fmt.Errorf("content is required for channel '%s'", channel)
		}
		for _, legacyContent := range contents {
			if legacyContent.present {
				return fmt.Errorf("%s must be nil for channel '%s'", legacyContent.name, channel)
			}
		}
		if contentSchemaVersion != ChannelTemplateContentSchemaVersion {
			return fmt.Errorf("content_schema_version must be %d for channel '%s'", ChannelTemplateContentSchemaVersion, channel)
		}
		return content.ValidateForChannel(channel)
	}
	if content != nil {
		return fmt.Errorf("content must be nil for channel '%s'", channel)
	}
	if contentSchemaVersion != 0 {
		return fmt.Errorf("content_schema_version must be zero for channel '%s'", channel)
	}
	for _, content := range contents {
		if content.name == channel && !content.present {
			return fmt.Errorf("%s is required for channel '%s'", content.name, channel)
		}
		if content.name != channel && content.present {
			return fmt.Errorf("%s must be nil for channel '%s'", content.name, channel)
		}
	}
	switch channel {
	case ChannelEmail:
		return email.Validate(testData)
	case ChannelWeb:
		return web.Validate(testData)
	case ChannelSMS:
		return sms.Validate(testData)
	case ChannelPush:
		return push.Validate(testData)
	default:
		return fmt.Errorf("unknown legacy template channel '%s'", channel)
	}
}

type PreviewTemplateRequest struct {
	WorkspaceID          string                         `json:"workspace_id"`
	Channel              string                         `json:"channel"`
	SMS                  *SMSTemplate                   `json:"sms,omitempty"`
	Push                 *PushTemplate                  `json:"push,omitempty"`
	Content              *ChannelTemplateContent        `json:"content,omitempty"`
	ContentSchemaVersion int                            `json:"content_schema_version,omitempty"`
	Translations         map[string]TemplateTranslation `json:"translations,omitempty"`
	Language             string                         `json:"language,omitempty"`
	Platform             string                         `json:"platform,omitempty"`
	Profile              string                         `json:"profile,omitempty"`
	TestData             MapOfAny                       `json:"test_data,omitempty"`
}

func (r *PreviewTemplateRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return fmt.Errorf("invalid preview template request: workspace_id is required")
	}
	if !IsRegisteredChannel(r.Channel) {
		return fmt.Errorf("invalid preview template request: unknown template channel '%s'", r.Channel)
	}
	if r.Language != "" && !IsValidLanguage(r.Language) {
		return fmt.Errorf("invalid preview template request: unsupported language '%s'", r.Language)
	}
	if isLegacyTemplateChannel(r.Channel) {
		if r.Channel != ChannelSMS && r.Channel != ChannelPush {
			return fmt.Errorf("invalid preview template request: legacy draft preview only supports '%s' or '%s'", ChannelSMS, ChannelPush)
		}
		if r.Platform != "" && r.Platform != EndpointPlatformAndroid && r.Platform != EndpointPlatformIOS && r.Platform != EndpointPlatformWeb {
			return fmt.Errorf("invalid preview template request: platform must be android, ios, or web")
		}
		if r.Channel == ChannelSMS && r.Platform != "" {
			return fmt.Errorf("invalid preview template request: platform is only supported for push")
		}
		if r.Channel == ChannelPush && r.Platform == "" {
			r.Platform = EndpointPlatformAndroid
		}
	} else {
		definition, _ := FindChannelDefinition(r.Channel)
		if r.Profile == "" && len(definition.PreviewProfiles) > 0 {
			r.Profile = definition.PreviewProfiles[0].ID
		}
		if !definitionSupportsProfile(definition, r.Profile) {
			return fmt.Errorf("invalid preview template request: preview profile '%s' is not supported by channel '%s'", r.Profile, r.Channel)
		}
		if r.Platform != "" {
			return fmt.Errorf("invalid preview template request: platform is only supported by push; use profile for channel '%s'", r.Channel)
		}
	}
	if err := validateTemplateContent(r.Channel, nil, nil, r.SMS, r.Push, r.Content, r.ContentSchemaVersion, r.TestData); err != nil {
		return fmt.Errorf("invalid preview template request: %w", err)
	}
	if err := validateTranslations(r.Translations, r.Channel, r.ContentSchemaVersion, r.TestData); err != nil {
		return fmt.Errorf("invalid preview template request: %w", err)
	}
	encoded, err := json.Marshal(r.TestData)
	if err != nil {
		return fmt.Errorf("invalid preview template request: test_data must be valid JSON: %w", err)
	}
	if len(encoded) > maxPreviewDataBytes {
		return fmt.Errorf("invalid preview template request: test_data must not exceed %d bytes", maxPreviewDataBytes)
	}
	return nil
}

func definitionSupportsProfile(definition ChannelDefinition, profileID string) bool {
	for _, profile := range definition.PreviewProfiles {
		if profile.ID == profileID {
			return true
		}
	}
	return false
}

type PreviewWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SMSPreview struct {
	Body           string `json:"body"`
	SenderID       string `json:"sender_id,omitempty"`
	Encoding       string `json:"encoding"`
	CharacterCount int    `json:"character_count"`
	UnitCount      int    `json:"unit_count"`
	SegmentCount   int    `json:"segment_count"`
	PerSegment     int    `json:"per_segment"`
	Remaining      int    `json:"remaining"`
}

type PushPreview struct {
	Title        string           `json:"title"`
	Body         string           `json:"body"`
	ImageURL     string           `json:"image_url,omitempty"`
	DeepLink     string           `json:"deep_link,omitempty"`
	Data         MapOfAny         `json:"data,omitempty"`
	Platform     string           `json:"platform"`
	PayloadBytes int              `json:"payload_bytes"`
	Warnings     []PreviewWarning `json:"warnings"`
}

type RenderedChannelMessage = ChannelTemplateContent

type GenericChannelPreview struct {
	Profile      string                 `json:"profile"`
	Direction    string                 `json:"direction"`
	PayloadBytes int                    `json:"payload_bytes"`
	Message      RenderedChannelMessage `json:"message"`
	Warnings     []PreviewWarning       `json:"warnings"`
}

type PreviewTemplateResponse struct {
	Channel           string                 `json:"channel"`
	RequestedLanguage string                 `json:"requested_language,omitempty"`
	ResolvedLanguage  string                 `json:"resolved_language"`
	FallbackUsed      bool                   `json:"fallback_used"`
	SMS               *SMSPreview            `json:"sms,omitempty"`
	Push              *PushPreview           `json:"push,omitempty"`
	ChannelPreview    *GenericChannelPreview `json:"channel_preview,omitempty"`
	TestData          MapOfAny               `json:"test_data,omitempty"`
}

func LanguageDirection(language string) string {
	base := strings.ToLower(strings.SplitN(language, "-", 2)[0])
	if base == "ar" || base == "he" || base == "ur" {
		return "rtl"
	}
	return "ltr"
}
