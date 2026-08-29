package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMSTemplateValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		err := (&SMSTemplate{Body: "Hi {{ contact.first_name }}", SenderID: "Notifuse"}).Validate(MapOfAny{})
		require.NoError(t, err)
	})

	t.Run("body required", func(t *testing.T) {
		err := (&SMSTemplate{}).Validate(MapOfAny{})
		require.ErrorContains(t, err, "body is required")
	})

	t.Run("bounded", func(t *testing.T) {
		err := (&SMSTemplate{Body: strings.Repeat("x", 10001)}).Validate(MapOfAny{})
		require.ErrorContains(t, err, "10000")
	})
}

func TestPushTemplateValidate(t *testing.T) {
	require.NoError(t, (&PushTemplate{
		Title:    "Order {{ order.id }}",
		Body:     "Your order shipped",
		ImageURL: "https://cdn.example.com/order.png",
		DeepLink: "notifuse://orders/42",
		Data:     MapOfAny{"order_id": "{{ order.id }}"},
	}).Validate(MapOfAny{}))

	require.ErrorContains(t, (&PushTemplate{Body: "body"}).Validate(nil), "title is required")
	require.ErrorContains(t, (&PushTemplate{Title: "title"}).Validate(nil), "body is required")
	require.ErrorContains(t, (&PushTemplate{Title: "title", Body: "body", ImageURL: "javascript:alert(1)"}).Validate(nil), "image_url")
}

func TestTemplateValidateSMSAndPushChannels(t *testing.T) {
	sms := &Template{
		ID: "sms-welcome", Name: "SMS welcome", Version: 1, Channel: ChannelSMS,
		Category: string(TemplateCategoryWelcome), SMS: &SMSTemplate{Body: "Welcome"},
		Translations: map[string]TemplateTranslation{"fr": {SMS: &SMSTemplate{Body: "Bienvenue"}}},
	}
	require.NoError(t, sms.Validate())

	push := &Template{
		ID: "push-order", Name: "Push order", Version: 1, Channel: ChannelPush,
		Category: string(TemplateCategoryTransactional), Push: &PushTemplate{Title: "Shipped", Body: "On its way"},
		Translations: map[string]TemplateTranslation{"fr": {Push: &PushTemplate{Title: "Expedie", Body: "En route"}}},
	}
	require.NoError(t, push.Validate())

	push.Email = &EmailTemplate{}
	require.ErrorContains(t, push.Validate(), "email must be nil")
}

func TestTemplateResolveSMSAndPushContent(t *testing.T) {
	template := &Template{
		SMS:  &SMSTemplate{Body: "Hello"},
		Push: &PushTemplate{Title: "Hello", Body: "Default"},
		Translations: map[string]TemplateTranslation{
			"fr": {
				SMS:  &SMSTemplate{Body: "Bonjour"},
				Push: &PushTemplate{Title: "Bonjour", Body: "Francais"},
			},
		},
	}
	assert.Equal(t, "Bonjour", template.ResolveSMSContent("fr", "en").Body)
	assert.Equal(t, "Hello", template.ResolveSMSContent("de", "en").Body)
	assert.Equal(t, "Bonjour", template.ResolvePushContent("fr", "en").Title)
	assert.Equal(t, "Hello", template.ResolvePushContent("en", "en").Title)
}

func TestPreviewTemplateRequestValidate(t *testing.T) {
	require.NoError(t, (&PreviewTemplateRequest{
		WorkspaceID: "ws-1", Channel: ChannelSMS, Language: "fr",
		SMS: &SMSTemplate{Body: "Bonjour {{ contact.first_name }}"},
	}).Validate())

	require.NoError(t, (&PreviewTemplateRequest{
		WorkspaceID: "ws-1", Channel: ChannelPush, Platform: EndpointPlatformIOS,
		Push: &PushTemplate{Title: "Hello", Body: "World"},
	}).Validate())

	require.ErrorContains(t, (&PreviewTemplateRequest{
		WorkspaceID: "ws-1", Channel: ChannelPush, Platform: "blackberry",
		Push: &PushTemplate{Title: "Hello", Body: "World"},
	}).Validate(), "platform")
}
