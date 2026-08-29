package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const maxChannelProviderResponseBytes = 64 * 1024

var deliveryE164Pattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

type ChannelProviderError struct {
	Provider   string
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}

func (e *ChannelProviderError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s provider error %s: %s", e.Provider, e.Code, e.Message)
	}
	return fmt.Sprintf("%s provider error: %s", e.Provider, e.Message)
}

func boundedProviderBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxChannelProviderResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxChannelProviderResponseBytes {
		return nil, fmt.Errorf("provider response exceeds %d bytes", maxChannelProviderResponseBytes)
	}
	return data, nil
}

func parseRetryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(strings.TrimSpace(value)); err == nil {
		if delay := time.Until(retryAt); delay > 0 {
			return delay
		}
	}
	return 0
}

type TwilioChannelProvider struct {
	settings domain.TwilioSettings
	client   *http.Client
	baseURL  string
}

func NewTwilioChannelProvider(settings domain.TwilioSettings, client *http.Client, baseURL string) (*TwilioChannelProvider, error) {
	if settings.AccountSID == "" || settings.AuthToken == "" {
		return nil, errors.New("twilio plaintext account_sid and auth_token are required")
	}
	if settings.FromNumber == "" && settings.MessagingServiceSID == "" {
		return nil, errors.New("twilio sender is required")
	}
	if settings.APIKeySID != "" && settings.APIKeySecret == "" {
		return nil, errors.New("twilio api key secret is required with api key sid")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://api.twilio.com"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("twilio base URL is invalid")
	}
	return &TwilioChannelProvider{settings: settings, client: client, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

func (p *TwilioChannelProvider) Send(ctx context.Context, request domain.ChannelDeliveryRequest) (*domain.ChannelDeliveryResult, error) {
	if request.Channel != domain.ChannelSMS || request.SMS == nil {
		return nil, errors.New("twilio requires sms channel content")
	}
	if err := request.SMS.Validate(nil); err != nil {
		return nil, err
	}
	if !deliveryE164Pattern.MatchString(request.Recipient) {
		return nil, errors.New("twilio recipient must be in E.164 format")
	}
	if strings.TrimSpace(request.EffectKey) == "" {
		return nil, errors.New("delivery effect key is required")
	}
	if request.StatusCallback != "" {
		parsed, err := url.ParseRequestURI(request.StatusCallback)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, errors.New("twilio status callback must be an absolute http or https URL")
		}
	}

	form := url.Values{}
	form.Set("To", request.Recipient)
	form.Set("Body", request.SMS.Body)
	if p.settings.MessagingServiceSID != "" {
		form.Set("MessagingServiceSid", p.settings.MessagingServiceSID)
	} else {
		form.Set("From", p.settings.FromNumber)
	}
	if request.StatusCallback != "" {
		form.Set("StatusCallback", request.StatusCallback)
	}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", p.baseURL, url.PathEscape(p.settings.AccountSID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	username, password := p.settings.AccountSID, p.settings.AuthToken
	if p.settings.APIKeySID != "" {
		username, password = p.settings.APIKeySID, p.settings.APIKeySecret
	}
	req.SetBasicAuth(username, password)

	response, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: twilio send outcome is unknown: %v", ErrSideEffectOutcomeUnknown, err)
	}
	defer response.Body.Close()
	body, err := boundedProviderBody(response.Body)
	if err != nil {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil, fmt.Errorf("%w: read twilio success response: %v", ErrSideEffectOutcomeUnknown, err)
		}
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Code    interface{} `json:"code"`
			Message string      `json:"message"`
		}
		_ = json.Unmarshal(body, &failure)
		message := strings.TrimSpace(failure.Message)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		code := ""
		if failure.Code != nil {
			code = fmt.Sprint(failure.Code)
		}
		return nil, &ChannelProviderError{
			Provider: "twilio", StatusCode: response.StatusCode, Code: code, Message: message,
			Retryable:  response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
		}
	}
	var accepted struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &accepted); err != nil {
		return nil, fmt.Errorf("%w: decode twilio success response: %v", ErrSideEffectOutcomeUnknown, err)
	}
	if accepted.SID == "" {
		return nil, fmt.Errorf("%w: twilio success response omitted message sid", ErrSideEffectOutcomeUnknown)
	}
	return &domain.ChannelDeliveryResult{Provider: "twilio", ProviderMessageID: accepted.SID, Status: accepted.Status}, nil
}

type FCMTokenSourceFactory func(context.Context, []byte) (oauth2.TokenSource, error)

type FCMChannelProvider struct {
	settings    domain.FCMSettings
	client      *http.Client
	baseURL     string
	tokenSource oauth2.TokenSource
}

func defaultFCMTokenSource(ctx context.Context, credentials []byte) (oauth2.TokenSource, error) {
	config, err := google.JWTConfigFromJSON(credentials, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, err
	}
	return config.TokenSource(ctx), nil
}

func NewFCMChannelProvider(settings domain.FCMSettings, client *http.Client, baseURL string, factory FCMTokenSourceFactory) (*FCMChannelProvider, error) {
	if settings.ProjectID == "" || settings.ServiceAccountJSON == "" {
		return nil, errors.New("fcm plaintext project_id and service_account_json are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://fcm.googleapis.com"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("fcm base URL is invalid")
	}
	if factory == nil {
		factory = defaultFCMTokenSource
	}
	tokenSource, err := factory(context.Background(), []byte(settings.ServiceAccountJSON))
	if err != nil {
		return nil, fmt.Errorf("create fcm token source: %w", err)
	}
	return &FCMChannelProvider{
		settings: settings, client: client, baseURL: strings.TrimRight(baseURL, "/"), tokenSource: oauth2.ReuseTokenSource(nil, tokenSource),
	}, nil
}

func (p *FCMChannelProvider) Send(ctx context.Context, request domain.ChannelDeliveryRequest) (*domain.ChannelDeliveryResult, error) {
	if request.Channel != domain.ChannelPush || request.Push == nil {
		return nil, errors.New("fcm requires push channel content")
	}
	if err := request.Push.Validate(nil); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Recipient) == "" {
		return nil, errors.New("fcm registration token is required")
	}
	if strings.TrimSpace(request.EffectKey) == "" {
		return nil, errors.New("delivery effect key is required")
	}
	data := make(map[string]string, len(request.Push.Data)+2)
	for key, value := range request.Push.Data {
		if stringValue, ok := value.(string); ok {
			data[key] = stringValue
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode fcm data %q: %w", key, err)
		}
		data[key] = string(encoded)
	}
	if request.Push.DeepLink != "" {
		data["_notifuse_deep_link"] = request.Push.DeepLink
	}
	data["_notifuse_effect_key"] = request.EffectKey
	notification := map[string]string{"title": request.Push.Title, "body": request.Push.Body}
	if request.Push.ImageURL != "" {
		notification["image"] = request.Push.ImageURL
	}
	payload := map[string]interface{}{
		"message": map[string]interface{}{
			"token": request.Recipient, "notification": notification, "data": data,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	token, err := p.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("get fcm oauth token: %w", err)
	}
	endpoint := fmt.Sprintf("%s/v1/projects/%s/messages:send", p.baseURL, url.PathEscape(p.settings.ProjectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fcm send outcome is unknown: %v", ErrSideEffectOutcomeUnknown, err)
	}
	defer response.Body.Close()
	responseBody, err := boundedProviderBody(response.Body)
	if err != nil {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil, fmt.Errorf("%w: read fcm success response: %v", ErrSideEffectOutcomeUnknown, err)
		}
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		_ = json.Unmarshal(responseBody, &failure)
		message := strings.TrimSpace(failure.Error.Message)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return nil, &ChannelProviderError{
			Provider: "fcm", StatusCode: response.StatusCode, Code: failure.Error.Status, Message: message,
			Retryable:  response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
		}
	}
	var accepted struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(responseBody, &accepted); err != nil {
		return nil, fmt.Errorf("%w: decode fcm success response: %v", ErrSideEffectOutcomeUnknown, err)
	}
	if accepted.Name == "" {
		return nil, fmt.Errorf("%w: fcm success response omitted message name", ErrSideEffectOutcomeUnknown)
	}
	return &domain.ChannelDeliveryResult{Provider: "fcm", ProviderMessageID: accepted.Name, Status: "accepted"}, nil
}
