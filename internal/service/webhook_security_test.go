package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/safehttpclient"
	"github.com/stretchr/testify/require"
)

func TestWebhookNetworkPolicyDoesNotRetireSubscription(t *testing.T) {
	t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", "false")
	ctrl := gomock.NewController(t)
	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	// No IncrementFailures or DisableWithReason call is allowed for policy refusal.
	deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), "ws", "delivery", gomock.Any(), 1, nil, gomock.Any(), gomock.Any()).Return(nil)
	worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, nil, logger.NewLogger(), nil)
	worker.processDelivery(context.Background(), "ws", &domain.WebhookDelivery{
		ID: "delivery", SubscriptionID: "subscription", EventType: "contact.created", MaxAttempts: 10,
		Payload: map[string]interface{}{"id": "customer"},
	}, &domain.WebhookSubscription{
		ID: "subscription", URL: "http://127.0.0.1:1/private", Enabled: true,
		Secret: "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
	}, time.Now())
}

func TestSignedChannelAndJourneyUseProtectedClientsByDefault(t *testing.T) {
	t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", "false")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("protected webhook reached a private destination")
	}))
	defer server.Close()
	provider, err := NewSignedWebhookChannelProvider(domain.ChannelWebhookSettings{
		EndpointURL: server.URL, Secret: "secret", Channels: []string{"telegram"}, TimeoutSeconds: 5,
	}, "telegram", nil, nil, nil)
	require.NoError(t, err)
	_, err = provider.Send(context.Background(), domain.ChannelDeliveryRequest{
		Channel: "telegram", Recipient: "recipient", EffectKey: "effect",
		Generic: &domain.RenderedChannelMessage{Family: domain.ContentFamilyText, Body: "test"},
	})
	require.ErrorIs(t, err, safehttpclient.ErrPrivateIP)
	require.NotErrorIs(t, err, ErrSideEffectOutcomeUnknown)
	_, err = NewWebhookNodeExecutor(logger.NewLogger()).httpClient.Get(server.URL)
	require.ErrorIs(t, err, safehttpclient.ErrPrivateIP)
	providerClient := &http.Client{Timeout: 11 * time.Second}
	resolver := NewWorkspaceChannelProviderResolver(providerClient)
	require.Same(t, providerClient, resolver.client, "dedicated provider transport must remain unchanged")
	_, err = resolver.webhookClient.Get(server.URL)
	require.ErrorIs(t, err, safehttpclient.ErrPrivateIP)
}

func TestWebhookSubscriptionRejectsPrivateDestinationBeforePersistence(t *testing.T) {
	t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", "false")
	require.ErrorIs(t, validateURL("http://169.254.169.254/metadata"), safehttpclient.ErrPrivateIP)
	require.NoError(t, validateURL("https://hooks.example.com/hook?key=value"))
}
