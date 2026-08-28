package domain

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

//go:generate mockgen -destination mocks/mock_mailjet_service.go -package mocks github.com/Notifuse/notifuse/internal/domain MailjetServiceInterface

// MailjetWebhookPayload represents the webhook payload from Mailjet
// According to https://dev.mailjet.com/email/guides/webhooks/
type MailjetWebhookPayload struct {
	Event     string `json:"event"`
	Time      int64  `json:"time"`
	MessageID int64  `json:"MessageID"`
	Email     string `json:"email"`

	// Message identification
	MjCampaignID int64  `json:"mj_campaign_id,omitempty"`
	MjContactID  int64  `json:"mj_contact_id,omitempty"`
	CustomID     string `json:"CustomID,omitempty"`
	MessageUUID  string `json:"MessageUUID,omitempty"`

	// Campaign information
	CustomCampaign string `json:"CustomCampaign,omitempty"`
	Payload        string `json:"Payload,omitempty"`

	// Bounce specific fields
	Blocked        bool   `json:"blocked,omitempty"`
	HardBounce     bool   `json:"hard_bounce,omitempty"`
	ErrorRelatedTo string `json:"error_related_to,omitempty"`
	Error          string `json:"error,omitempty"`

	// Complaint specific fields
	Source string `json:"source,omitempty"`

	// Common fields
	Comment string `json:"comment,omitempty"`

	// Click/Open specific fields
	URL       string `json:"url,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

// MailjetWebhook represents a webhook configuration in Mailjet
// According to https://dev.mailjet.com/email/reference/webhook#v3_post_eventcallbackurl
type MailjetWebhook struct {
	ID        int64  `json:"ID,omitempty"`
	APIKey    string `json:"APIKey,omitempty"`
	Endpoint  string `json:"Url"`
	EventType string `json:"EventType"`
	Status    string `json:"Status,omitempty"`
	Version   int    `json:"Version"`
	IsBackup  bool   `json:"IsBackup,omitempty"`
}

// MailjetWebhookEventType represents the available event types for webhooks
type MailjetWebhookEventType string

const (
	// Event types as defined in Mailjet documentation
	MailjetEventSent    MailjetWebhookEventType = "sent"
	MailjetEventBounce  MailjetWebhookEventType = "bounce"
	MailjetEventBlocked MailjetWebhookEventType = "blocked"
	MailjetEventSpam    MailjetWebhookEventType = "spam"
	MailjetEventUnsub   MailjetWebhookEventType = "unsub"
	MailjetEventClick   MailjetWebhookEventType = "click"
	MailjetEventOpen    MailjetWebhookEventType = "open"
)

// MailjetWebhookResponse represents a response for webhook operations
type MailjetWebhookResponse struct {
	Count int              `json:"Count"`
	Data  []MailjetWebhook `json:"Data"`
	Total int              `json:"Total"`
}

// MailjetSettings contains configuration for Mailjet
type MailjetSettings struct {
	EncryptedAPIKey    string `json:"encrypted_api_key,omitempty"`
	EncryptedSecretKey string `json:"encrypted_secret_key,omitempty"`
	SandboxMode        bool   `json:"sandbox_mode"`

	// decoded keys, not stored in the database
	APIKey    string `json:"api_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
}

func (m *MailjetSettings) DecryptAPIKey(passphrase string) error {
	apiKey, err := crypto.DecryptFromHexString(m.EncryptedAPIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Mailjet API key: %w", err)
	}
	m.APIKey = apiKey
	return nil
}

func (m *MailjetSettings) EncryptAPIKey(passphrase string) error {
	encryptedAPIKey, err := crypto.EncryptString(m.APIKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Mailjet API key: %w", err)
	}
	m.EncryptedAPIKey = encryptedAPIKey
	return nil
}

func (m *MailjetSettings) DecryptSecretKey(passphrase string) error {
	secretKey, err := crypto.DecryptFromHexString(m.EncryptedSecretKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to decrypt Mailjet Secret key: %w", err)
	}
	m.SecretKey = secretKey
	return nil
}

func (m *MailjetSettings) EncryptSecretKey(passphrase string) error {
	encryptedSecretKey, err := crypto.EncryptString(m.SecretKey, passphrase)
	if err != nil {
		return fmt.Errorf("failed to encrypt Mailjet Secret key: %w", err)
	}
	m.EncryptedSecretKey = encryptedSecretKey
	return nil
}

func (m *MailjetSettings) Validate(passphrase string) error {
	// API Key is required for Mailjet
	if m.APIKey != "" {
		if err := m.EncryptAPIKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt Mailjet API key: %w", err)
		}
		m.APIKey = "" // Clear the API key after encryption
	}

	// Secret Key is required for Mailjet
	if m.SecretKey != "" {
		if err := m.EncryptSecretKey(passphrase); err != nil {
			return fmt.Errorf("failed to encrypt Mailjet Secret key: %w", err)
		}
		m.SecretKey = "" // Clear the Secret key after encryption
	}

	return nil
}

//go:generate mockgen -destination mocks/mock_mailjet_service.go -package mocks github.com/Notifuse/notifuse/internal/domain MailjetServiceInterface

// MailjetServiceInterface defines operations for managing Mailjet webhooks
type MailjetServiceInterface interface {
	// ListWebhooks retrieves all registered webhooks
	ListWebhooks(ctx context.Context, config MailjetSettings) (*MailjetWebhookResponse, error)

	// CreateWebhook creates a new webhook
	CreateWebhook(ctx context.Context, config MailjetSettings, webhook MailjetWebhook) (*MailjetWebhook, error)

	// GetWebhook retrieves a webhook by ID
	GetWebhook(ctx context.Context, config MailjetSettings, webhookID int64) (*MailjetWebhook, error)

	// UpdateWebhook updates an existing webhook
	UpdateWebhook(ctx context.Context, config MailjetSettings, webhookID int64, webhook MailjetWebhook) (*MailjetWebhook, error)

	// DeleteWebhook deletes a webhook by ID
	DeleteWebhook(ctx context.Context, config MailjetSettings, webhookID int64) error
}
