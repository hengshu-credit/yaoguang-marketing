package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTemplateContentValidatesRealChannelCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		content ChannelTemplateContent
	}{
		{
			name:    "telegram text with liquid",
			channel: "telegram",
			content: ChannelTemplateContent{Family: ContentFamilyText, Body: "Hello {{ contact.first_name }}"},
		},
		{
			name:    "whatsapp approved external template",
			channel: "whatsapp",
			content: ChannelTemplateContent{Family: ContentFamilyExternalTemplate, ExternalTemplate: &ExternalTemplateReference{ID: "order_update", Language: "es_MX", Parameters: []TemplateParameterBinding{{Name: "order_id", Value: "{{ data.order_id }}"}}}},
		},
		{
			name:    "line rich card",
			channel: "line",
			content: ChannelTemplateContent{Family: ContentFamilyRichCard, Title: "Member offer", Body: "Save today", Media: &ChannelMedia{Type: "image", URL: "https://cdn.example/offer.png"}, Actions: []ChannelAction{{Type: "url", Label: "Open", Value: "https://example.com/offer"}}},
		},
		{
			name:    "webhook json payload",
			channel: "webhook",
			content: ChannelTemplateContent{Family: ContentFamilyWebhookPayload, Webhook: &WebhookPayloadTemplate{ContentType: "application/json", Body: `{"customer_id":"{{ customer.id }}"}`}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, test.content.ValidateForChannel(test.channel))
		})
	}
}

func TestChannelTemplateContentRejectsUnsupportedOrUnsafeShapes(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		content ChannelTemplateContent
		want    string
	}{
		{name: "unknown channel", channel: "unknown", content: ChannelTemplateContent{Family: ContentFamilyText, Body: "hello"}, want: "unknown template channel"},
		{name: "unsupported family", channel: "webhook", content: ChannelTemplateContent{Family: ContentFamilyCarousel, Cards: []ChannelCard{{Title: "one"}, {Title: "two"}}}, want: "does not support content family"},
		{name: "blank text", channel: "telegram", content: ChannelTemplateContent{Family: ContentFamilyText}, want: "body is required"},
		{name: "insecure media", channel: "line", content: ChannelTemplateContent{Family: ContentFamilyRichCard, Body: "offer", Media: &ChannelMedia{Type: "image", URL: "http://cdn.example/offer.png"}}, want: "media url must be an absolute https URL"},
		{name: "action has no target", channel: "line", content: ChannelTemplateContent{Family: ContentFamilyRichCard, Body: "offer", Actions: []ChannelAction{{Type: "url", Label: "Open"}}}, want: "action value is required"},
		{name: "external template missing id", channel: "whatsapp", content: ChannelTemplateContent{Family: ContentFamilyExternalTemplate, ExternalTemplate: &ExternalTemplateReference{Language: "es_MX"}}, want: "external template id is required"},
		{name: "body over channel limit", channel: "messenger", content: ChannelTemplateContent{Family: ContentFamilyText, Body: strings.Repeat("界", 2001)}, want: "body must not exceed 2000 characters"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.content.ValidateForChannel(test.channel)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestGenericTemplateValidationKeepsLegacyAndGenericContentExclusive(t *testing.T) {
	legacy := validSMSTemplateFixture()
	require.NoError(t, legacy.Validate())

	generic := &Template{
		ID: "telegram-welcome", Name: "Telegram welcome", Version: 1,
		Channel: "telegram", Category: string(TemplateCategoryMarketing),
		ContentSchemaVersion: ChannelTemplateContentSchemaVersion,
		Content:              &ChannelTemplateContent{Family: ContentFamilyText, Body: "Welcome"},
	}
	require.NoError(t, generic.Validate())

	generic.SMS = &SMSTemplate{Body: "must not coexist"}
	err := generic.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sms must be nil for channel 'telegram'")
}

func TestGenericTemplateTranslationMustMatchTheTemplateChannel(t *testing.T) {
	template := &Template{
		ID: "line-offer", Name: "LINE offer", Version: 1,
		Channel: "line", Category: string(TemplateCategoryMarketing),
		ContentSchemaVersion: ChannelTemplateContentSchemaVersion,
		Content:              &ChannelTemplateContent{Family: ContentFamilyText, Body: "Default"},
		Translations: map[string]TemplateTranslation{
			"th": {Content: &ChannelTemplateContent{Family: ContentFamilyText, Body: "ข้อเสนอ"}},
		},
	}
	require.NoError(t, template.Validate())

	template.Translations["th"] = TemplateTranslation{SMS: &SMSTemplate{Body: "wrong channel"}}
	err := template.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content is required for channel 'line'")
}

func TestPreviewTemplateRequestAcceptsRegisteredGenericChannelAndProfile(t *testing.T) {
	request := PreviewTemplateRequest{
		WorkspaceID:          "workspace-1",
		Channel:              "telegram",
		ContentSchemaVersion: ChannelTemplateContentSchemaVersion,
		Content:              &ChannelTemplateContent{Family: ContentFamilyText, Body: "Hello {{ customer.name }}"},
		Profile:              "telegram_mobile",
		Language:             "ur",
		TestData:             MapOfAny{"customer": MapOfAny{"name": "Ali"}},
	}
	require.NoError(t, request.Validate())

	request.Profile = "whatsapp_web"
	err := request.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preview profile 'whatsapp_web' is not supported by channel 'telegram'")
}

func TestResolveChannelContentReturnsTranslationAndFallback(t *testing.T) {
	defaultContent := &ChannelTemplateContent{Family: ContentFamilyText, Body: "Default"}
	translated := &ChannelTemplateContent{Family: ContentFamilyText, Body: "ข้อเสนอ"}
	template := &Template{
		Content: defaultContent,
		Translations: map[string]TemplateTranslation{
			"th": {Content: translated},
		},
	}

	content, resolved, fallback := template.ResolveChannelContent("th", "en")
	assert.Same(t, translated, content)
	assert.Equal(t, "th", resolved)
	assert.False(t, fallback)

	content, resolved, fallback = template.ResolveChannelContent("vi", "en")
	assert.Same(t, defaultContent, content)
	assert.Equal(t, "en", resolved)
	assert.True(t, fallback)
}

func validSMSTemplateFixture() *Template {
	return &Template{
		ID: "sms-welcome", Name: "SMS welcome", Version: 1,
		Channel: ChannelSMS, Category: string(TemplateCategoryMarketing),
		SMS: &SMSTemplate{Body: "Welcome"},
	}
}
