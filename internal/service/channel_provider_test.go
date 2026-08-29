package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type staticTokenSource struct{ token *oauth2.Token }

func (s staticTokenSource) Token() (*oauth2.Token, error) { return s.token, nil }

func plaintextTwilioSettings() domain.TwilioSettings {
	return domain.TwilioSettings{
		AccountSID: "AC" + "0123456789abcdef0123456789abcdef",
		AuthToken:  "auth-secret", APIKeySID: "SK" + "0123456789abcdef0123456789abcdef",
		APIKeySecret: "api-secret", FromNumber: "+15551234567",
	}
}

func TestTwilioChannelProviderSendsSMS(t *testing.T) {
	var captured map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/2010-04-01/Accounts/AC" + "0123456789abcdef0123456789abcdef/Messages.json", r.URL.Path)
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "SK" + "0123456789abcdef0123456789abcdef", username)
		assert.Equal(t, "api-secret", password)
		require.NoError(t, r.ParseForm())
		captured = map[string]string{
			"To": r.Form.Get("To"), "From": r.Form.Get("From"), "Body": r.Form.Get("Body"),
			"StatusCallback": r.Form.Get("StatusCallback"),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sid":"SM0123456789abcdef0123456789abcdef","status":"queued"}`))
	}))
	defer server.Close()

	provider, err := NewTwilioChannelProvider(plaintextTwilioSettings(), server.Client(), server.URL)
	require.NoError(t, err)
	result, err := provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: domain.ChannelSMS, Recipient: "+15557654321",
		SMS: &domain.SMSTemplate{Body: "Hello"}, StatusCallback: "https://notify.example/webhooks/delivery/twilio?workspace_id=ws1",
		EffectKey: "effect-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "twilio", result.Provider)
	assert.Equal(t, "SM0123456789abcdef0123456789abcdef", result.ProviderMessageID)
	assert.Equal(t, "queued", result.Status)
	assert.Equal(t, "+15557654321", captured["To"])
	assert.Equal(t, "+15551234567", captured["From"])
	assert.Equal(t, "Hello", captured["Body"])
	assert.Contains(t, captured["StatusCallback"], "workspace_id=ws1")
}

func TestTwilioChannelProviderClassifies429AsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":20429,"message":"Too many requests"}`))
	}))
	defer server.Close()
	provider, err := NewTwilioChannelProvider(plaintextTwilioSettings(), server.Client(), server.URL)
	require.NoError(t, err)
	_, err = provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: domain.ChannelSMS, Recipient: "+15557654321", SMS: &domain.SMSTemplate{Body: "Hello"}, EffectKey: "effect-1",
	})
	var providerErr *ChannelProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.True(t, providerErr.Retryable)
	assert.Equal(t, http.StatusTooManyRequests, providerErr.StatusCode)
	assert.Equal(t, "20429", providerErr.Code)
}

func TestTwilioChannelProviderTreatsMalformedSuccessAsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()
	provider, err := NewTwilioChannelProvider(plaintextTwilioSettings(), server.Client(), server.URL)
	require.NoError(t, err)
	_, err = provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: domain.ChannelSMS, Recipient: "+15557654321", SMS: &domain.SMSTemplate{Body: "Hello"}, EffectKey: "effect-1",
	})
	assert.ErrorIs(t, err, ErrSideEffectOutcomeUnknown)
}

func TestFCMChannelProviderSendsHTTPV1Push(t *testing.T) {
	var payload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/projects/notifuse-test/messages:send", r.URL.Path)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/notifuse-test/messages/message-1"}`))
	}))
	defer server.Close()

	settings := domain.FCMSettings{ProjectID: "notifuse-test", ServiceAccountJSON: `{"type":"service_account"}`}
	provider, err := NewFCMChannelProvider(settings, server.Client(), server.URL, func(context.Context, []byte) (oauth2.TokenSource, error) {
		return staticTokenSource{token: &oauth2.Token{AccessToken: "test-access-token"}}, nil
	})
	require.NoError(t, err)
	result, err := provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: domain.ChannelPush, Recipient: "device-token",
		Push: &domain.PushTemplate{
			Title: "Order update", Body: "Shipped", ImageURL: "https://example.com/image.png",
			DeepLink: "notifuse://orders/42", Data: domain.MapOfAny{"order_id": 42, "vip": true},
		},
		EffectKey: "effect-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "fcm", result.Provider)
	assert.Equal(t, "projects/notifuse-test/messages/message-1", result.ProviderMessageID)

	message := payload["message"].(map[string]interface{})
	assert.Equal(t, "device-token", message["token"])
	notification := message["notification"].(map[string]interface{})
	assert.Equal(t, "Order update", notification["title"])
	data := message["data"].(map[string]interface{})
	assert.Equal(t, "42", data["order_id"])
	assert.Equal(t, "true", data["vip"])
	assert.Equal(t, "notifuse://orders/42", data["_notifuse_deep_link"])
	assert.Equal(t, "effect-1", data["_notifuse_effect_key"])
}

func TestFCMChannelProviderTreatsMalformedSuccessAsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()
	settings := domain.FCMSettings{ProjectID: "notifuse-test", ServiceAccountJSON: `{"type":"service_account"}`}
	provider, err := NewFCMChannelProvider(settings, server.Client(), server.URL, func(context.Context, []byte) (oauth2.TokenSource, error) {
		return staticTokenSource{token: &oauth2.Token{AccessToken: "test-access-token"}}, nil
	})
	require.NoError(t, err)
	_, err = provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: domain.ChannelPush, Recipient: "device-token",
		Push: &domain.PushTemplate{Title: "Order update", Body: "Shipped"}, EffectKey: "effect-1",
	})
	assert.ErrorIs(t, err, ErrSideEffectOutcomeUnknown)
}
