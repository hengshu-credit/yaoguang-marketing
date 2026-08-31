package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type channelWebhookEnvelope struct {
	SchemaVersion   int                            `json:"schema_version"`
	Event           string                         `json:"event"`
	EffectKey       string                         `json:"effect_key"`
	Channel         string                         `json:"channel"`
	Recipient       string                         `json:"recipient"`
	Platform        string                         `json:"platform,omitempty"`
	Locale          string                         `json:"locale,omitempty"`
	TemplateID      string                         `json:"template_id,omitempty"`
	TemplateVersion int64                          `json:"template_version,omitempty"`
	Message         *domain.RenderedChannelMessage `json:"message"`
	Metadata        domain.MapOfAny                `json:"metadata,omitempty"`
}

type channelWebhookResponse struct {
	Status            string `json:"status"`
	ProviderMessageID string `json:"provider_message_id,omitempty"`
	Code              string `json:"code,omitempty"`
	Message           string `json:"message,omitempty"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

type SignedWebhookChannelProvider struct {
	settings domain.ChannelWebhookSettings
	channel  string
	client   *http.Client
	now      func() time.Time
	nonce    func() string
}

func NewSignedWebhookChannelProvider(
	settings domain.ChannelWebhookSettings,
	channel string,
	client *http.Client,
	now func() time.Time,
	nonce func() string,
) (*SignedWebhookChannelProvider, error) {
	parsed, err := url.Parse(settings.EndpointURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackWebhookHost(parsed.Hostname()))) {
		return nil, errors.New("channel Webhook endpoint must be absolute HTTPS")
	}
	if settings.Secret == "" {
		return nil, errors.New("channel Webhook plaintext secret is required")
	}
	if !containsChannel(settings.Channels, channel) {
		return nil, fmt.Errorf("channel Webhook integration does not allow channel %s", channel)
	}
	if client == nil {
		client = &http.Client{}
	}
	clientCopy := *client
	clientCopy.Timeout = time.Duration(settings.TimeoutSeconds) * time.Second
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("channel Webhook redirects are not allowed")
	}
	if now == nil {
		now = time.Now
	}
	if nonce == nil {
		nonce = uuid.NewString
	}
	return &SignedWebhookChannelProvider{settings: settings, channel: channel, client: &clientCopy, now: now, nonce: nonce}, nil
}

func isLoopbackWebhookHost(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func containsChannel(channels []string, expected string) bool {
	for _, channel := range channels {
		if channel == expected {
			return true
		}
	}
	return false
}

func SignChannelWebhook(secret string, timestamp int64, nonce string, body []byte) string {
	canonical := strconv.FormatInt(timestamp, 10) + "." + nonce + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func (p *SignedWebhookChannelProvider) Send(ctx context.Context, delivery domain.ChannelDeliveryRequest) (*domain.ChannelDeliveryResult, error) {
	if delivery.Channel != p.channel {
		return nil, fmt.Errorf("channel Webhook provider for %s cannot send %s", p.channel, delivery.Channel)
	}
	if delivery.Generic == nil {
		return nil, errors.New("channel Webhook requires rendered generic content")
	}
	body, err := json.Marshal(channelWebhookEnvelope{
		SchemaVersion: 1, Event: "channel.delivery", EffectKey: delivery.EffectKey,
		Channel: delivery.Channel, Recipient: delivery.Recipient, Platform: delivery.Platform,
		Locale: delivery.Locale, TemplateID: delivery.TemplateID, TemplateVersion: delivery.TemplateVersion,
		Message: delivery.Generic, Metadata: delivery.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("encode channel Webhook envelope: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.settings.EndpointURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create channel Webhook request: %w", err)
	}
	timestamp := p.now().UTC().Unix()
	nonce := p.nonce()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Yaoguang-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("X-Yaoguang-Nonce", nonce)
	request.Header.Set("X-Yaoguang-Effect-Key", delivery.EffectKey)
	request.Header.Set("X-Yaoguang-Signature", SignChannelWebhook(p.settings.Secret, timestamp, nonce, body))
	for key, value := range p.settings.Headers {
		request.Header.Set(key, value)
	}

	response, err := p.client.Do(request)
	if err != nil {
		if strings.Contains(err.Error(), "redirects are not allowed") {
			return nil, err
		}
		return nil, fmt.Errorf("%w: channel Webhook outcome is unknown: %v", ErrSideEffectOutcomeUnknown, err)
	}
	defer response.Body.Close()
	responseBody, err := boundedProviderBody(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read channel Webhook response: %w", err)
	}
	var result channelWebhookResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("decode channel Webhook response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &ChannelProviderError{
			Provider: "channel_webhook", StatusCode: response.StatusCode, Code: result.Code,
			Message: result.Message, Retryable: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
		}
	}
	switch result.Status {
	case "accepted":
		return &domain.ChannelDeliveryResult{Provider: "channel_webhook", ProviderMessageID: result.ProviderMessageID, Status: result.Status}, nil
	case "rejected":
		return nil, &ChannelProviderError{Provider: "channel_webhook", Code: result.Code, Message: result.Message}
	case "retryable":
		return nil, &ChannelProviderError{Provider: "channel_webhook", Code: result.Code, Message: result.Message, Retryable: true, RetryAfter: time.Duration(result.RetryAfterSeconds) * time.Second}
	default:
		return nil, fmt.Errorf("%w: channel Webhook returned unknown status %q", ErrSideEffectOutcomeUnknown, result.Status)
	}
}
