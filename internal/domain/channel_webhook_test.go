package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelWebhookSettingsValidateAndEncryptSecret(t *testing.T) {
	settings := &ChannelWebhookSettings{
		EndpointURL:    "https://bridge.example/send",
		Secret:         "plain-secret",
		Channels:       []string{"telegram", "zalo"},
		TimeoutSeconds: 5,
		Headers:        map[string]string{"X-Bridge-Tenant": "north"},
	}
	require.NoError(t, settings.Validate("master-key"))
	assert.Empty(t, settings.Secret)
	assert.NotEmpty(t, settings.EncryptedSecret)
	require.NoError(t, settings.DecryptSecretKeys("master-key"))
	assert.Equal(t, "plain-secret", settings.Secret)
}

func TestChannelWebhookSettingsRejectUnsafeOrUnsupportedConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		settings ChannelWebhookSettings
		want     string
	}{
		{name: "http endpoint", settings: ChannelWebhookSettings{EndpointURL: "http://bridge.example/send", Secret: "secret", Channels: []string{"telegram"}, TimeoutSeconds: 5}, want: "endpoint_url must use https"},
		{name: "embedded credentials", settings: ChannelWebhookSettings{EndpointURL: "https://user:pass@bridge.example/send", Secret: "secret", Channels: []string{"telegram"}, TimeoutSeconds: 5}, want: "must not contain credentials"},
		{name: "native-only channel", settings: ChannelWebhookSettings{EndpointURL: "https://bridge.example/send", Secret: "secret", Channels: []string{"email"}, TimeoutSeconds: 5}, want: "channel 'email' does not support signed Webhook delivery"},
		{name: "unknown channel", settings: ChannelWebhookSettings{EndpointURL: "https://bridge.example/send", Secret: "secret", Channels: []string{"future"}, TimeoutSeconds: 5}, want: "unknown channel 'future'"},
		{name: "duplicate channel", settings: ChannelWebhookSettings{EndpointURL: "https://bridge.example/send", Secret: "secret", Channels: []string{"telegram", "telegram"}, TimeoutSeconds: 5}, want: "duplicate channel 'telegram'"},
		{name: "reserved header", settings: ChannelWebhookSettings{EndpointURL: "https://bridge.example/send", Secret: "secret", Channels: []string{"telegram"}, TimeoutSeconds: 5, Headers: map[string]string{"Authorization": "secret"}}, want: "header 'Authorization' is reserved"},
		{name: "timeout too low", settings: ChannelWebhookSettings{EndpointURL: "https://bridge.example/send", Secret: "secret", Channels: []string{"telegram"}, TimeoutSeconds: 0}, want: "timeout_seconds must be between 1 and 30"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.settings.Validate("master-key")
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestChannelWebhookIntegrationUsesWorkspaceSecretLifecycle(t *testing.T) {
	integration := &Integration{
		ID: "bridge-1", Name: "Regional bridge", Type: IntegrationTypeChannelWebhook,
		ChannelWebhookSettings: &ChannelWebhookSettings{
			EndpointURL: "https://bridge.example/send", Secret: "plain-secret",
			Channels: []string{"telegram"}, TimeoutSeconds: 5,
		},
	}
	require.NoError(t, integration.Validate("master-key"))
	require.NoError(t, integration.BeforeSave("master-key"))
	assert.Empty(t, integration.ChannelWebhookSettings.Secret)
	assert.NotEmpty(t, integration.ChannelWebhookSettings.EncryptedSecret)
	require.NoError(t, integration.AfterLoad("master-key"))
	assert.Equal(t, "plain-secret", integration.ChannelWebhookSettings.Secret)
}

func TestChannelWebhookIntegrationRedactsPlaintextAndCiphertextWithHint(t *testing.T) {
	integration := &Integration{
		Type: IntegrationTypeChannelWebhook,
		ChannelWebhookSettings: &ChannelWebhookSettings{
			Secret: "regional-secret", EncryptedSecret: "ciphertext",
		},
	}
	integration.Redact()
	assert.Empty(t, integration.ChannelWebhookSettings.Secret)
	assert.Empty(t, integration.ChannelWebhookSettings.EncryptedSecret)
	assert.Equal(t, "cret", integration.CredentialHints["channel_webhook.secret"])
}
