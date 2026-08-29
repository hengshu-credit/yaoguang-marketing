package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const channelProviderTestSecret = "test-global-secret-key-with-32-bytes"

func validTwilioProvider() *SMSProvider {
	return &SMSProvider{
		Kind: SMSProviderKindTwilio,
		Twilio: &TwilioSettings{
			AccountSID:   "AC" + "0123456789abcdef0123456789abcdef",
			AuthToken:    "twilio-auth-token-secret",
			APIKeySID:    "SK" + "0123456789abcdef0123456789abcdef",
			APIKeySecret: "twilio-api-key-secret",
			FromNumber:   "+15551234567",
		},
	}
}

func validFCMProvider() *PushProvider {
	return &PushProvider{
		Kind: PushProviderKindFCM,
		FCM: &FCMSettings{
			ProjectID:          "notifuse-test",
			ServiceAccountJSON: `{"type":"service_account","project_id":"notifuse-test","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n","client_email":"notifuse@notifuse-test.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`,
		},
	}
}

func TestSMSProviderValidateEncryptsTwilioSecrets(t *testing.T) {
	provider := validTwilioProvider()
	require.NoError(t, provider.Validate(channelProviderTestSecret))
	require.NotNil(t, provider.Twilio)
	assert.Empty(t, provider.Twilio.AuthToken)
	assert.Empty(t, provider.Twilio.APIKeySecret)
	assert.NotEmpty(t, provider.Twilio.EncryptedAuthToken)
	assert.NotEmpty(t, provider.Twilio.EncryptedAPIKeySecret)

	require.NoError(t, provider.DecryptSecretKeys(channelProviderTestSecret))
	assert.Equal(t, "twilio-auth-token-secret", provider.Twilio.AuthToken)
	assert.Equal(t, "twilio-api-key-secret", provider.Twilio.APIKeySecret)
}

func TestSMSProviderRequiresOneSenderAndCompleteAPIKeyPair(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TwilioSettings)
		want   string
	}{
		{name: "no sender", mutate: func(s *TwilioSettings) { s.FromNumber = "" }, want: "from_number or messaging_service_sid"},
		{name: "two senders", mutate: func(s *TwilioSettings) { s.MessagingServiceSID = "MG0123456789abcdef0123456789abcdef" }, want: "exactly one"},
		{name: "api key sid without secret", mutate: func(s *TwilioSettings) { s.APIKeySecret = "" }, want: "api_key_sid and api_key_secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := validTwilioProvider()
			tt.mutate(provider.Twilio)
			assert.ErrorContains(t, provider.Validate(channelProviderTestSecret), tt.want)
		})
	}
}

func TestPushProviderValidateEncryptsServiceAccount(t *testing.T) {
	provider := validFCMProvider()
	require.NoError(t, provider.Validate(channelProviderTestSecret))
	require.NotNil(t, provider.FCM)
	assert.Empty(t, provider.FCM.ServiceAccountJSON)
	assert.NotEmpty(t, provider.FCM.EncryptedServiceAccountJSON)

	require.NoError(t, provider.DecryptSecretKeys(channelProviderTestSecret))
	assert.Contains(t, provider.FCM.ServiceAccountJSON, `"project_id":"notifuse-test"`)
}

func TestPushProviderRejectsMismatchedServiceAccountProject(t *testing.T) {
	provider := validFCMProvider()
	provider.FCM.ServiceAccountJSON = strings.ReplaceAll(provider.FCM.ServiceAccountJSON, "notifuse-test", "other-project")
	assert.ErrorContains(t, provider.Validate(channelProviderTestSecret), "project_id")
}

func TestIntegrationValidatesSMSAndPushProviderTypes(t *testing.T) {
	sms := &Integration{ID: "sms-1", Name: "Twilio", Type: IntegrationTypeSMS, SMSProvider: validTwilioProvider()}
	assert.NoError(t, sms.Validate(channelProviderTestSecret))

	push := &Integration{ID: "push-1", Name: "FCM", Type: IntegrationTypePush, PushProvider: validFCMProvider()}
	assert.NoError(t, push.Validate(channelProviderTestSecret))

	missing := &Integration{ID: "sms-2", Name: "Missing", Type: IntegrationTypeSMS}
	assert.ErrorContains(t, missing.Validate(channelProviderTestSecret), "sms provider settings are required")
}

func TestUpdateIntegrationAllowsRedactedChannelCredentials(t *testing.T) {
	sms := &UpdateIntegrationRequest{
		WorkspaceID: "ws-1", IntegrationID: "sms-1", Name: "Twilio",
		SMSProvider: &SMSProvider{Kind: SMSProviderKindTwilio, Twilio: &TwilioSettings{
			AccountSID: "AC" + "0123456789abcdef0123456789abcdef", APIKeySID: "SK" + "0123456789abcdef0123456789abcdef",
			FromNumber: "+15551234567",
		}},
	}
	assert.NoError(t, sms.Validate(channelProviderTestSecret))

	push := &UpdateIntegrationRequest{
		WorkspaceID: "ws-1", IntegrationID: "push-1", Name: "FCM",
		PushProvider: &PushProvider{Kind: PushProviderKindFCM, FCM: &FCMSettings{ProjectID: "notifuse-test"}},
	}
	assert.NoError(t, push.Validate(channelProviderTestSecret))
}

func TestIntegrationRedactRemovesChannelSecretsAndAddsHints(t *testing.T) {
	integration := Integration{
		ID: "sms-1", Name: "Twilio", Type: IntegrationTypeSMS, SMSProvider: validTwilioProvider(),
	}
	require.NoError(t, integration.SMSProvider.Validate(channelProviderTestSecret))
	require.NoError(t, integration.SMSProvider.DecryptSecretKeys(channelProviderTestSecret))
	integration.Redact()

	assert.Empty(t, integration.SMSProvider.Twilio.AuthToken)
	assert.Empty(t, integration.SMSProvider.Twilio.APIKeySecret)
	assert.Equal(t, "cret", integration.CredentialHints["twilio.auth_token"])
	assert.Equal(t, "cret", integration.CredentialHints["twilio.api_key_secret"])
	assert.Empty(t, integration.SMSProvider.Twilio.EncryptedAuthToken)
	assert.Empty(t, integration.SMSProvider.Twilio.EncryptedAPIKeySecret)

	push := Integration{ID: "push-1", Name: "FCM", Type: IntegrationTypePush, PushProvider: validFCMProvider()}
	require.NoError(t, push.PushProvider.Validate(channelProviderTestSecret))
	require.NoError(t, push.PushProvider.DecryptSecretKeys(channelProviderTestSecret))
	push.Redact()
	assert.Empty(t, push.PushProvider.FCM.ServiceAccountJSON)
	assert.Empty(t, push.PushProvider.FCM.EncryptedServiceAccountJSON)
	assert.NotEmpty(t, push.CredentialHints["fcm.service_account_json"])
}
