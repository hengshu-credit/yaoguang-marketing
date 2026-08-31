package service

import (
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderGenericChannelPreviewRendersLiquidAcrossStructuredContent(t *testing.T) {
	content := &domain.ChannelTemplateContent{
		Family: domain.ContentFamilyRichCard,
		Title:  "Hello {{ customer.name }}",
		Body:   "Order {{ order.id }} is ready",
		Media:  &domain.ChannelMedia{Type: "image", URL: "https://cdn.example/{{ order.id }}.png", AltText: "Order {{ order.id }}"},
		Actions: []domain.ChannelAction{
			{Type: "url", Label: "Open {{ order.id }}", Value: "https://shop.example/orders/{{ order.id }}"},
		},
		Data: domain.MapOfAny{"tracking": domain.MapOfAny{"order_id": "{{ order.id }}"}},
	}
	preview, err := renderGenericChannelPreview(
		content,
		"line",
		"line_mobile",
		"ltr",
		domain.MapOfAny{"customer": domain.MapOfAny{"name": "Ada"}, "order": domain.MapOfAny{"id": "42"}},
	)
	require.NoError(t, err)
	assert.Equal(t, "line_mobile", preview.Profile)
	assert.Equal(t, "ltr", preview.Direction)
	assert.Equal(t, "Hello Ada", preview.Message.Title)
	assert.Equal(t, "Order 42 is ready", preview.Message.Body)
	assert.Equal(t, "https://cdn.example/42.png", preview.Message.Media.URL)
	assert.Equal(t, "Open 42", preview.Message.Actions[0].Label)
	nested, ok := preview.Message.Data["tracking"].(domain.MapOfAny)
	require.True(t, ok)
	assert.Equal(t, "42", nested["order_id"])
	assert.Positive(t, preview.PayloadBytes)
}

func TestRenderGenericChannelPreviewRejectsProfileFromAnotherChannel(t *testing.T) {
	_, err := renderGenericChannelPreview(
		&domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "Hello"},
		"telegram",
		"whatsapp_web",
		"ltr",
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preview profile 'whatsapp_web' is not supported by channel 'telegram'")
}

func TestRenderGenericChannelPreviewPreservesRTLDirection(t *testing.T) {
	preview, err := renderGenericChannelPreview(
		&domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "خوش آمدید"},
		"telegram",
		"telegram_mobile",
		"rtl",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "rtl", preview.Direction)
}
