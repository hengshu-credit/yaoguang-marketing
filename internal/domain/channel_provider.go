package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

type SMSProviderKind string
type PushProviderKind string

const (
	SMSProviderKindTwilio SMSProviderKind  = "twilio"
	PushProviderKindFCM   PushProviderKind = "fcm"
)

var (
	twilioAccountSIDPattern   = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	twilioAPIKeySIDPattern    = regexp.MustCompile(`^SK[0-9a-fA-F]{32}$`)
	twilioMessagingSIDPattern = regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`)
	e164Pattern               = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	firebaseProjectIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
)

type TwilioSettings struct {
	AccountSID            string `json:"account_sid"`
	AuthToken             string `json:"auth_token,omitempty"`
	EncryptedAuthToken    string `json:"encrypted_auth_token,omitempty"`
	APIKeySID             string `json:"api_key_sid,omitempty"`
	APIKeySecret          string `json:"api_key_secret,omitempty"`
	EncryptedAPIKeySecret string `json:"encrypted_api_key_secret,omitempty"`
	FromNumber            string `json:"from_number,omitempty"`
	MessagingServiceSID   string `json:"messaging_service_sid,omitempty"`
}

type SMSProvider struct {
	Kind   SMSProviderKind `json:"kind"`
	Twilio *TwilioSettings `json:"twilio,omitempty"`
}

func (p *SMSProvider) Validate(passphrase string) error {
	return p.validate(passphrase, true)
}

func (p *SMSProvider) ValidateForUpdate(passphrase string) error {
	return p.validate(passphrase, false)
}

func (p *SMSProvider) validate(passphrase string, requireSecrets bool) error {
	if p == nil || p.Kind == "" {
		return errors.New("sms provider kind is required")
	}
	if p.Kind != SMSProviderKindTwilio {
		return fmt.Errorf("invalid sms provider kind: %s", p.Kind)
	}
	if p.Twilio == nil {
		return errors.New("twilio settings are required")
	}
	return p.Twilio.validate(passphrase, requireSecrets)
}

func (s *TwilioSettings) Validate(passphrase string) error {
	return s.validate(passphrase, true)
}

func (s *TwilioSettings) validate(passphrase string, requireSecrets bool) error {
	if s == nil {
		return errors.New("twilio settings are required")
	}
	if !twilioAccountSIDPattern.MatchString(s.AccountSID) {
		return errors.New("twilio account_sid must match AC followed by 32 hexadecimal characters")
	}
	if requireSecrets && s.AuthToken == "" && s.EncryptedAuthToken == "" {
		return errors.New("twilio auth_token is required")
	}
	hasAPIKeySID := s.APIKeySID != ""
	hasAPIKeySecret := s.APIKeySecret != "" || s.EncryptedAPIKeySecret != ""
	if (!hasAPIKeySID && hasAPIKeySecret) || (requireSecrets && hasAPIKeySID != hasAPIKeySecret) {
		return errors.New("twilio api_key_sid and api_key_secret must be configured together")
	}
	if hasAPIKeySID && !twilioAPIKeySIDPattern.MatchString(s.APIKeySID) {
		return errors.New("twilio api_key_sid must match SK followed by 32 hexadecimal characters")
	}
	hasFrom := s.FromNumber != ""
	hasMessagingService := s.MessagingServiceSID != ""
	if !hasFrom && !hasMessagingService {
		return errors.New("twilio from_number or messaging_service_sid is required")
	}
	if hasFrom && hasMessagingService {
		return errors.New("twilio must configure exactly one of from_number or messaging_service_sid")
	}
	if hasFrom && !e164Pattern.MatchString(s.FromNumber) {
		return errors.New("twilio from_number must be in E.164 format")
	}
	if hasMessagingService && !twilioMessagingSIDPattern.MatchString(s.MessagingServiceSID) {
		return errors.New("twilio messaging_service_sid must match MG followed by 32 hexadecimal characters")
	}
	return s.encryptSecrets(passphrase)
}

func (s *TwilioSettings) encryptSecrets(passphrase string) error {
	if s.AuthToken != "" {
		encrypted, err := crypto.EncryptString(s.AuthToken, passphrase)
		if err != nil {
			return fmt.Errorf("encrypt twilio auth token: %w", err)
		}
		s.EncryptedAuthToken = encrypted
		s.AuthToken = ""
	}
	if s.APIKeySecret != "" {
		encrypted, err := crypto.EncryptString(s.APIKeySecret, passphrase)
		if err != nil {
			return fmt.Errorf("encrypt twilio api key secret: %w", err)
		}
		s.EncryptedAPIKeySecret = encrypted
		s.APIKeySecret = ""
	}
	return nil
}

func (p *SMSProvider) EncryptSecretKeys(passphrase string) error {
	if p == nil || p.Twilio == nil {
		return nil
	}
	return p.Twilio.encryptSecrets(passphrase)
}

func (p *SMSProvider) DecryptSecretKeys(passphrase string) error {
	if p == nil || p.Twilio == nil {
		return nil
	}
	if p.Twilio.EncryptedAuthToken != "" {
		value, err := crypto.DecryptFromHexString(p.Twilio.EncryptedAuthToken, passphrase)
		if err != nil {
			return fmt.Errorf("decrypt twilio auth token: %w", err)
		}
		p.Twilio.AuthToken = value
	}
	if p.Twilio.EncryptedAPIKeySecret != "" {
		value, err := crypto.DecryptFromHexString(p.Twilio.EncryptedAPIKeySecret, passphrase)
		if err != nil {
			return fmt.Errorf("decrypt twilio api key secret: %w", err)
		}
		p.Twilio.APIKeySecret = value
	}
	return nil
}

type FCMSettings struct {
	ProjectID                   string `json:"project_id"`
	ServiceAccountJSON          string `json:"service_account_json,omitempty"`
	EncryptedServiceAccountJSON string `json:"encrypted_service_account_json,omitempty"`
}

type PushProvider struct {
	Kind PushProviderKind `json:"kind"`
	FCM  *FCMSettings     `json:"fcm,omitempty"`
}

func (p *PushProvider) Validate(passphrase string) error {
	return p.validate(passphrase, true)
}

func (p *PushProvider) ValidateForUpdate(passphrase string) error {
	return p.validate(passphrase, false)
}

func (p *PushProvider) validate(passphrase string, requireSecrets bool) error {
	if p == nil || p.Kind == "" {
		return errors.New("push provider kind is required")
	}
	if p.Kind != PushProviderKindFCM {
		return fmt.Errorf("invalid push provider kind: %s", p.Kind)
	}
	if p.FCM == nil {
		return errors.New("fcm settings are required")
	}
	return p.FCM.validate(passphrase, requireSecrets)
}

func (s *FCMSettings) Validate(passphrase string) error {
	return s.validate(passphrase, true)
}

func (s *FCMSettings) validate(passphrase string, requireSecrets bool) error {
	if s == nil {
		return errors.New("fcm settings are required")
	}
	if !firebaseProjectIDPattern.MatchString(s.ProjectID) {
		return errors.New("fcm project_id is invalid")
	}
	if requireSecrets && s.ServiceAccountJSON == "" && s.EncryptedServiceAccountJSON == "" {
		return errors.New("fcm service_account_json is required")
	}
	if s.ServiceAccountJSON != "" {
		var credential struct {
			Type        string `json:"type"`
			ProjectID   string `json:"project_id"`
			PrivateKey  string `json:"private_key"`
			ClientEmail string `json:"client_email"`
			TokenURI    string `json:"token_uri"`
		}
		if err := json.Unmarshal([]byte(s.ServiceAccountJSON), &credential); err != nil {
			return fmt.Errorf("fcm service_account_json is invalid: %w", err)
		}
		if credential.Type != "service_account" || credential.ProjectID != s.ProjectID ||
			strings.TrimSpace(credential.PrivateKey) == "" || strings.TrimSpace(credential.ClientEmail) == "" {
			return errors.New("fcm service_account_json must contain matching project_id, client_email and private_key")
		}
		if credential.TokenURI == "" {
			return errors.New("fcm service_account_json token_uri is required")
		}
	}
	return s.encryptSecret(passphrase)
}

func (s *FCMSettings) encryptSecret(passphrase string) error {
	if s.ServiceAccountJSON == "" {
		return nil
	}
	encrypted, err := crypto.EncryptString(s.ServiceAccountJSON, passphrase)
	if err != nil {
		return fmt.Errorf("encrypt fcm service account: %w", err)
	}
	s.EncryptedServiceAccountJSON = encrypted
	s.ServiceAccountJSON = ""
	return nil
}

func (p *PushProvider) EncryptSecretKeys(passphrase string) error {
	if p == nil || p.FCM == nil {
		return nil
	}
	return p.FCM.encryptSecret(passphrase)
}

func (p *PushProvider) DecryptSecretKeys(passphrase string) error {
	if p == nil || p.FCM == nil || p.FCM.EncryptedServiceAccountJSON == "" {
		return nil
	}
	value, err := crypto.DecryptFromHexString(p.FCM.EncryptedServiceAccountJSON, passphrase)
	if err != nil {
		return fmt.Errorf("decrypt fcm service account: %w", err)
	}
	p.FCM.ServiceAccountJSON = value
	return nil
}

type ChannelDeliveryRequest struct {
	Channel        string
	Recipient      string
	SMS            *SMSTemplate
	Push           *PushTemplate
	StatusCallback string
	EffectKey      string
}

type ChannelDeliveryResult struct {
	Provider          string
	ProviderMessageID string
	Status            string
}

type ChannelProvider interface {
	Send(context.Context, ChannelDeliveryRequest) (*ChannelDeliveryResult, error)
}
