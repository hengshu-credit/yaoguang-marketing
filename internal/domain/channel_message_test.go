package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendChannelMessageRequestValidateNormalizesIdentity(t *testing.T) {
	request := &SendChannelMessageRequest{
		WorkspaceID: " ws-1 ", EffectKey: " order-42:sms ", Channel: " SMS ",
		IntegrationID: " twilio-main ", ContactEmail: " USER@Example.COM ",
		EndpointID: " phone-primary ", TemplateID: " order-ready ", Language: " en ",
		Data: MapOfAny{"order_id": "42"},
	}
	require.NoError(t, request.Validate())
	assert.Equal(t, "ws-1", request.WorkspaceID)
	assert.Equal(t, "order-42:sms", request.EffectKey)
	assert.Equal(t, ChannelSMS, request.Channel)
	assert.Equal(t, "user@example.com", request.ContactEmail)
}

func TestSendChannelMessageRequestValidateRequiresStableEffectKey(t *testing.T) {
	request := &SendChannelMessageRequest{
		WorkspaceID: "ws-1", Channel: ChannelPush, IntegrationID: "fcm-main",
		ContactEmail: "user@example.com", TemplateID: "order-ready",
	}
	assert.EqualError(t, request.Validate(), "effect_key must contain 1 to 255 characters")
}

func TestSendChannelMessageRequestHashIsStableAndSemantic(t *testing.T) {
	first := &SendChannelMessageRequest{
		WorkspaceID: "ws-1", EffectKey: "effect-1", Channel: ChannelPush,
		IntegrationID: "fcm-main", ContactEmail: "user@example.com", TemplateID: "ready",
		Data: MapOfAny{"nested": MapOfAny{"b": 2, "a": 1}},
	}
	second := &SendChannelMessageRequest{
		WorkspaceID: "ws-1", EffectKey: "effect-1", Channel: ChannelPush,
		IntegrationID: "fcm-main", ContactEmail: "user@example.com", TemplateID: "ready",
		Data: MapOfAny{"nested": MapOfAny{"a": 1, "b": 2}},
	}
	require.NoError(t, first.Validate())
	require.NoError(t, second.Validate())
	firstHash, err := first.RequestHash()
	require.NoError(t, err)
	secondHash, err := second.RequestHash()
	require.NoError(t, err)
	assert.Equal(t, firstHash, secondHash)

	second.Language = "fr"
	secondHash, err = second.RequestHash()
	require.NoError(t, err)
	assert.NotEqual(t, firstHash, secondHash)
}
