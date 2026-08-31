package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignedWebhookChannelProviderSignsExactBodyAndMapsAcceptance(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		expectedCanonical := strconv.FormatInt(now.Unix(), 10) + ".nonce-1." + string(receivedBody)
		mac := hmac.New(sha256.New, []byte("plain-secret"))
		_, _ = mac.Write([]byte(expectedCanonical))
		assert.Equal(t, "v1="+hex.EncodeToString(mac.Sum(nil)), r.Header.Get("X-Yaoguang-Signature"))
		assert.Equal(t, "nonce-1", r.Header.Get("X-Yaoguang-Nonce"))
		assert.Equal(t, "effect-1", r.Header.Get("X-Yaoguang-Effect-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"accepted","provider_message_id":"provider-1"}`))
	}))
	defer server.Close()

	provider, err := NewSignedWebhookChannelProvider(domain.ChannelWebhookSettings{
		EndpointURL: server.URL, Secret: "plain-secret", Channels: []string{"telegram"}, TimeoutSeconds: 5,
	}, "telegram", server.Client(), func() time.Time { return now }, func() string { return "nonce-1" })
	require.NoError(t, err)
	result, err := provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: "telegram", Recipient: "chat-123", EffectKey: "effect-1", Platform: "telegram_mobile", Locale: "kk",
		TemplateID: "welcome", TemplateVersion: 2,
		Generic: &domain.RenderedChannelMessage{Family: domain.ContentFamilyText, Body: "Hello"},
	})
	require.NoError(t, err)
	assert.Equal(t, "channel_webhook", result.Provider)
	assert.Equal(t, "provider-1", result.ProviderMessageID)
	assert.Equal(t, "accepted", result.Status)
	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(receivedBody, &envelope))
	assert.Equal(t, "telegram", envelope["channel"])
	assert.Equal(t, "chat-123", envelope["recipient"])
}

func TestSignedWebhookChannelProviderRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	provider, err := NewSignedWebhookChannelProvider(
		domain.ChannelWebhookSettings{EndpointURL: redirect.URL, Secret: "secret", Channels: []string{"telegram"}, TimeoutSeconds: 5},
		"telegram", redirect.Client(), time.Now, func() string { return "nonce" },
	)
	require.NoError(t, err)
	_, err = provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: "telegram", Recipient: "chat", EffectKey: "effect",
		Generic: &domain.RenderedChannelMessage{Family: domain.ContentFamilyText, Body: "Hello"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirects are not allowed")
}
