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
	"unicode/utf8"
)

type ChannelSendStatus string

const (
	ChannelSendReserved  ChannelSendStatus = "reserved"
	ChannelSendSubmitted ChannelSendStatus = "submitted"
	ChannelSendConfirmed ChannelSendStatus = "confirmed"
	ChannelSendFailed    ChannelSendStatus = "failed"
	ChannelSendUnknown   ChannelSendStatus = "unknown"
)

var ErrChannelSendHashConflict = errors.New("effect key already exists with a different channel send request")

type SendChannelMessageRequest struct {
	WorkspaceID     string   `json:"workspace_id"`
	EffectKey       string   `json:"effect_key"`
	Channel         string   `json:"channel"`
	IntegrationID   string   `json:"integration_id"`
	ContactEmail    string   `json:"contact_email"`
	EndpointID      string   `json:"endpoint_id,omitempty"`
	TemplateID      string   `json:"template_id"`
	TemplateVersion int64    `json:"template_version,omitempty"`
	Language        string   `json:"language,omitempty"`
	Data            MapOfAny `json:"data,omitempty"`
	Metadata        MapOfAny `json:"metadata,omitempty"`
}

func (r *SendChannelMessageRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("request is required")
	}
	r.WorkspaceID = strings.TrimSpace(r.WorkspaceID)
	r.EffectKey = strings.TrimSpace(r.EffectKey)
	r.Channel = strings.ToLower(strings.TrimSpace(r.Channel))
	r.IntegrationID = strings.TrimSpace(r.IntegrationID)
	r.ContactEmail = NormalizeEmail(r.ContactEmail)
	r.EndpointID = strings.TrimSpace(r.EndpointID)
	r.TemplateID = strings.TrimSpace(r.TemplateID)
	r.Language = strings.TrimSpace(r.Language)
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.EffectKey == "" || utf8.RuneCountInString(r.EffectKey) > 255 {
		return fmt.Errorf("effect_key must contain 1 to 255 characters")
	}
	if r.Channel != ChannelSMS && r.Channel != ChannelPush {
		return fmt.Errorf("channel must be sms or push")
	}
	if r.IntegrationID == "" || utf8.RuneCountInString(r.IntegrationID) > 255 {
		return fmt.Errorf("integration_id must contain 1 to 255 characters")
	}
	contact := &Contact{Email: r.ContactEmail}
	if err := contact.Validate(); err != nil {
		return fmt.Errorf("invalid contact_email")
	}
	if utf8.RuneCountInString(r.EndpointID) > 128 {
		return fmt.Errorf("endpoint_id cannot exceed 128 characters")
	}
	if r.TemplateID == "" || utf8.RuneCountInString(r.TemplateID) > 255 {
		return fmt.Errorf("template_id must contain 1 to 255 characters")
	}
	if r.TemplateVersion < 0 {
		return fmt.Errorf("template_version cannot be negative")
	}
	if r.Language != "" && !IsValidLanguage(r.Language) {
		return fmt.Errorf("unsupported language '%s'", r.Language)
	}
	for label, value := range map[string]MapOfAny{"data": r.Data, "metadata": r.Metadata} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("%s must be valid JSON: %w", label, err)
		}
		limit := 128 * 1024
		if label == "metadata" {
			limit = 16 * 1024
		}
		if len(encoded) > limit {
			return fmt.Errorf("%s cannot exceed %d bytes", label, limit)
		}
	}
	return nil
}

func (r SendChannelMessageRequest) RequestHash() (string, error) {
	// encoding/json sorts map keys, producing a stable hash for semantically equal objects.
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("encode channel send request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type ChannelSendExecution struct {
	EffectKey         string            `json:"effect_key"`
	RequestHash       string            `json:"request_hash"`
	MessageID         string            `json:"message_id"`
	Channel           string            `json:"channel"`
	IntegrationID     string            `json:"integration_id"`
	ContactEmail      string            `json:"contact_email"`
	EndpointID        string            `json:"endpoint_id"`
	TemplateID        string            `json:"template_id"`
	TemplateVersion   int64             `json:"template_version"`
	Language          string            `json:"language,omitempty"`
	Status            ChannelSendStatus `json:"status"`
	Provider          string            `json:"provider,omitempty"`
	ProviderMessageID string            `json:"provider_message_id,omitempty"`
	Attempts          int               `json:"attempts"`
	LastError         string            `json:"last_error,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type SendChannelMessageResponse struct {
	Execution ChannelSendExecution `json:"execution"`
	Duplicate bool                 `json:"duplicate"`
}

type ChannelSendRepository interface {
	Reserve(context.Context, string, ChannelSendExecution) (ChannelSendExecution, bool, error)
	MarkSubmitted(context.Context, string, string, time.Time) (bool, error)
	Confirm(context.Context, string, string, string, string, string, *MessageHistory, time.Time) (bool, error)
	Fail(context.Context, string, string, ChannelSendStatus, string, time.Time) (bool, error)
}

type ChannelMessageService interface {
	Send(context.Context, *SendChannelMessageRequest) (*SendChannelMessageResponse, error)
}
