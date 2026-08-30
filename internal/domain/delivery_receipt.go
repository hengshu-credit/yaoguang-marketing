package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxDeliveryReceiptMetadataBytes = 16 * 1024

type DeliveryProvider string

const (
	DeliveryProviderTwilio    DeliveryProvider = "twilio"
	DeliveryProviderFCM       DeliveryProvider = "fcm"
	DeliveryProviderSMTP      DeliveryProvider = "smtp"
	DeliveryProviderSES       DeliveryProvider = "ses"
	DeliveryProviderSparkPost DeliveryProvider = "sparkpost"
	DeliveryProviderPostmark  DeliveryProvider = "postmark"
	DeliveryProviderMailgun   DeliveryProvider = "mailgun"
	DeliveryProviderMailjet   DeliveryProvider = "mailjet"
	DeliveryProviderSendGrid  DeliveryProvider = "sendgrid"
)

type DeliveryReceiptEvent string

const (
	DeliveryReceiptAccepted   DeliveryReceiptEvent = "accepted"
	DeliveryReceiptSent       DeliveryReceiptEvent = "sent"
	DeliveryReceiptDelivered  DeliveryReceiptEvent = "delivered"
	DeliveryReceiptOpened     DeliveryReceiptEvent = "opened"
	DeliveryReceiptClicked    DeliveryReceiptEvent = "clicked"
	DeliveryReceiptBounced    DeliveryReceiptEvent = "bounced"
	DeliveryReceiptComplained DeliveryReceiptEvent = "complained"
	DeliveryReceiptFailed     DeliveryReceiptEvent = "failed"
)

type DeliveryReceipt struct {
	Provider          DeliveryProvider       `json:"provider"`
	ReceiptID         string                 `json:"receipt_id"`
	ProviderMessageID string                 `json:"provider_message_id,omitempty"`
	MessageID         string                 `json:"message_id,omitempty"`
	EffectKey         string                 `json:"effect_key,omitempty"`
	Event             DeliveryReceiptEvent   `json:"event"`
	OccurredAt        time.Time              `json:"occurred_at"`
	ReceivedAt        time.Time              `json:"received_at,omitempty"`
	ErrorCode         string                 `json:"error_code,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	PayloadHash       string                 `json:"payload_hash,omitempty"`
}

func (r *DeliveryReceipt) Validate() error {
	if r == nil {
		return errors.New("receipt is required")
	}
	switch r.Provider {
	case DeliveryProviderTwilio, DeliveryProviderFCM, DeliveryProviderSMTP, DeliveryProviderSES,
		DeliveryProviderSparkPost, DeliveryProviderPostmark, DeliveryProviderMailgun,
		DeliveryProviderMailjet, DeliveryProviderSendGrid:
	default:
		return errors.New("provider is not supported")
	}
	if strings.TrimSpace(r.ReceiptID) == "" || len(r.ReceiptID) > 255 {
		return errors.New("receipt_id is required and must not exceed 255 characters")
	}
	if len(r.ProviderMessageID) > 255 || len(r.MessageID) > 255 || len(r.EffectKey) > 255 {
		return errors.New("provider_message_id, message_id and effect_key must not exceed 255 characters")
	}
	if strings.TrimSpace(r.ProviderMessageID) == "" && strings.TrimSpace(r.MessageID) == "" && strings.TrimSpace(r.EffectKey) == "" {
		return errors.New("provider_message_id or message_id or effect_key is required")
	}
	switch r.Event {
	case DeliveryReceiptAccepted, DeliveryReceiptSent, DeliveryReceiptDelivered, DeliveryReceiptOpened,
		DeliveryReceiptClicked, DeliveryReceiptBounced, DeliveryReceiptComplained, DeliveryReceiptFailed:
	default:
		return errors.New("event is invalid")
	}
	if r.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if len(r.ErrorCode) > 255 {
		return errors.New("error_code must not exceed 255 characters")
	}
	metadata, err := json.Marshal(r.Metadata)
	if err != nil {
		return fmt.Errorf("metadata must be valid JSON: %w", err)
	}
	if len(metadata) > MaxDeliveryReceiptMetadataBytes {
		return fmt.Errorf("metadata must not exceed %d bytes", MaxDeliveryReceiptMetadataBytes)
	}
	return nil
}

// ComputePayloadHash fingerprints only provider-supplied business data. The
// server-assigned ReceivedAt and PayloadHash fields are deliberately excluded.
func (r DeliveryReceipt) ComputePayloadHash() (string, error) {
	payload, err := json.Marshal(struct {
		Provider          DeliveryProvider       `json:"provider"`
		ReceiptID         string                 `json:"receipt_id"`
		ProviderMessageID string                 `json:"provider_message_id,omitempty"`
		MessageID         string                 `json:"message_id,omitempty"`
		EffectKey         string                 `json:"effect_key,omitempty"`
		Event             DeliveryReceiptEvent   `json:"event"`
		OccurredAt        time.Time              `json:"occurred_at"`
		ErrorCode         string                 `json:"error_code,omitempty"`
		Metadata          map[string]interface{} `json:"metadata,omitempty"`
	}{
		Provider: r.Provider, ReceiptID: r.ReceiptID, ProviderMessageID: r.ProviderMessageID,
		MessageID: r.MessageID, EffectKey: r.EffectKey, Event: r.Event,
		OccurredAt: r.OccurredAt.UTC(), ErrorCode: r.ErrorCode, Metadata: r.Metadata,
	})
	if err != nil {
		return "", fmt.Errorf("marshal delivery receipt fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

type IngestDeliveryReceiptsRequest struct {
	WorkspaceID string            `json:"workspace_id"`
	Receipts    []DeliveryReceipt `json:"receipts"`
}

func (r *IngestDeliveryReceiptsRequest) ValidateEnvelope(maxBatchSize int) error {
	if r == nil {
		return errors.New("request is required")
	}
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	if len(r.Receipts) == 0 {
		return errors.New("at least one receipt is required")
	}
	if maxBatchSize <= 0 {
		return errors.New("max batch size must be positive")
	}
	if len(r.Receipts) > maxBatchSize {
		return fmt.Errorf("receipts must contain at most %d items", maxBatchSize)
	}
	return nil
}

type DeliveryReceiptRecordResult struct {
	Provider  DeliveryProvider `json:"provider"`
	ReceiptID string           `json:"receipt_id"`
	MessageID string           `json:"message_id,omitempty"`
	IntentID  string           `json:"intent_id,omitempty"`
	AttemptID string           `json:"attempt_id,omitempty"`
	Duplicate bool             `json:"duplicate"`
	Matched   bool             `json:"matched"`
	Applied   bool             `json:"applied"`
	Conflict  bool             `json:"conflict"`
}

type IngestDeliveryReceiptResult struct {
	DeliveryReceiptRecordResult
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type IngestDeliveryReceiptsResponse struct {
	Accepted   int                           `json:"accepted"`
	Duplicates int                           `json:"duplicates"`
	Conflicts  int                           `json:"conflicts"`
	Failed     int                           `json:"failed"`
	Results    []IngestDeliveryReceiptResult `json:"results"`
}

var ErrDeliveryReceiptPayloadConflict = errors.New("delivery receipt id was reused with a different payload")

type DeliveryReceiptRepository interface {
	RecordBatch(context.Context, string, []DeliveryReceipt) ([]DeliveryReceiptRecordResult, error)
}

type TwilioDeliveryCallback struct {
	WorkspaceID   string
	IntegrationID string
	CallbackURL   string
	Signature     string
	MessageID     string
	EffectKey     string
	Form          map[string][]string
}

type DeliveryReceiptService interface {
	Ingest(context.Context, *IngestDeliveryReceiptsRequest) (*IngestDeliveryReceiptsResponse, error)
	ProcessTwilioCallback(context.Context, TwilioDeliveryCallback) (*DeliveryReceiptRecordResult, error)
}
