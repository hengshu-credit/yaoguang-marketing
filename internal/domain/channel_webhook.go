package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

type ChannelWebhookSettings struct {
	EndpointURL     string            `json:"endpoint_url"`
	Secret          string            `json:"secret,omitempty"`
	EncryptedSecret string            `json:"encrypted_secret,omitempty"`
	Channels        []string          `json:"channels"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
	Headers         map[string]string `json:"headers,omitempty"`
}

func (s *ChannelWebhookSettings) Validate(passphrase string) error {
	return s.validate(passphrase, true)
}

func (s *ChannelWebhookSettings) ValidateForUpdate(passphrase string) error {
	return s.validate(passphrase, false)
}

func (s *ChannelWebhookSettings) validate(passphrase string, requireSecret bool) error {
	if s == nil {
		return errors.New("channel Webhook settings are required")
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(s.EndpointURL))
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return errors.New("endpoint_url must use https and be absolute")
	}
	if parsed.User != nil {
		return errors.New("endpoint_url must not contain credentials")
	}
	if requireSecret && strings.TrimSpace(s.Secret) == "" && s.EncryptedSecret == "" {
		return errors.New("secret is required")
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 30 {
		return errors.New("timeout_seconds must be between 1 and 30")
	}
	if len(s.Channels) == 0 {
		return errors.New("at least one channel is required")
	}
	seen := make(map[string]struct{}, len(s.Channels))
	for index, rawChannel := range s.Channels {
		channel := strings.ToLower(strings.TrimSpace(rawChannel))
		definition, ok := FindChannelDefinition(channel)
		if !ok {
			return fmt.Errorf("unknown channel '%s'", channel)
		}
		if _, duplicate := seen[channel]; duplicate {
			return fmt.Errorf("duplicate channel '%s'", channel)
		}
		if !containsString(definition.DeliveryModes, ChannelDeliveryModeSignedWebhook) {
			return fmt.Errorf("channel '%s' does not support signed Webhook delivery", channel)
		}
		seen[channel] = struct{}{}
		s.Channels[index] = channel
	}
	if len(s.Headers) > 20 {
		return errors.New("headers must not exceed 20 entries")
	}
	for key, value := range s.Headers {
		if isReservedChannelWebhookHeader(key) {
			return fmt.Errorf("header '%s' is reserved", key)
		}
		if strings.TrimSpace(key) == "" || utf8.RuneCountInString(key) > 128 {
			return errors.New("header names must contain 1 to 128 characters")
		}
		if utf8.RuneCountInString(value) > 2048 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("header '%s' has an invalid value", key)
		}
	}
	return s.EncryptSecretKeys(passphrase)
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func isReservedChannelWebhookHeader(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(normalized, "x-yaoguang-") {
		return true
	}
	switch normalized {
	case "authorization", "cookie", "host", "content-type", "content-length", "user-agent":
		return true
	default:
		return false
	}
}

func (s *ChannelWebhookSettings) EncryptSecretKeys(passphrase string) error {
	if s == nil || s.Secret == "" {
		return nil
	}
	encrypted, err := crypto.EncryptString(s.Secret, passphrase)
	if err != nil {
		return fmt.Errorf("encrypt channel Webhook secret: %w", err)
	}
	s.EncryptedSecret = encrypted
	s.Secret = ""
	return nil
}

func (s *ChannelWebhookSettings) DecryptSecretKeys(passphrase string) error {
	if s == nil || s.EncryptedSecret == "" {
		return nil
	}
	secret, err := crypto.DecryptFromHexString(s.EncryptedSecret, passphrase)
	if err != nil {
		return fmt.Errorf("decrypt channel Webhook secret: %w", err)
	}
	s.Secret = secret
	return nil
}
