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

	"golang.org/x/text/unicode/norm"
)

type DeliveryStatus string

const (
	DeliveryStatusPlanned          DeliveryStatus = "planned"
	DeliveryStatusReserved         DeliveryStatus = "reserved"
	DeliveryStatusQueued           DeliveryStatus = "queued"
	DeliveryStatusSubmitting       DeliveryStatus = "submitting"
	DeliveryStatusProviderAccepted DeliveryStatus = "provider_accepted"
	DeliveryStatusConfirmed        DeliveryStatus = "confirmed"
	DeliveryStatusSuppressed       DeliveryStatus = "suppressed"
	DeliveryStatusDeferred         DeliveryStatus = "deferred"
	DeliveryStatusTransientFailed  DeliveryStatus = "transient_failed"
	DeliveryStatusTerminalFailed   DeliveryStatus = "terminal_failed"
	DeliveryStatusUnknown          DeliveryStatus = "unknown"
	DeliveryStatusCancelled        DeliveryStatus = "cancelled"
)

var deliveryStatusTransitions = map[DeliveryStatus]map[DeliveryStatus]struct{}{
	DeliveryStatusPlanned: {
		DeliveryStatusReserved: {}, DeliveryStatusSuppressed: {}, DeliveryStatusDeferred: {}, DeliveryStatusCancelled: {},
	},
	DeliveryStatusReserved: {
		DeliveryStatusQueued: {}, DeliveryStatusSuppressed: {}, DeliveryStatusDeferred: {}, DeliveryStatusCancelled: {},
	},
	DeliveryStatusQueued: {
		DeliveryStatusSubmitting: {}, DeliveryStatusDeferred: {}, DeliveryStatusTransientFailed: {}, DeliveryStatusCancelled: {},
	},
	DeliveryStatusSubmitting: {
		DeliveryStatusProviderAccepted: {}, DeliveryStatusTransientFailed: {}, DeliveryStatusTerminalFailed: {}, DeliveryStatusUnknown: {},
	},
	DeliveryStatusProviderAccepted: {
		DeliveryStatusConfirmed: {}, DeliveryStatusTerminalFailed: {}, DeliveryStatusUnknown: {},
	},
	DeliveryStatusDeferred: {
		DeliveryStatusQueued: {}, DeliveryStatusCancelled: {},
	},
	DeliveryStatusTransientFailed: {
		DeliveryStatusQueued: {}, DeliveryStatusTerminalFailed: {}, DeliveryStatusCancelled: {},
	},
	DeliveryStatusUnknown: {
		DeliveryStatusProviderAccepted: {}, DeliveryStatusConfirmed: {}, DeliveryStatusTerminalFailed: {}, DeliveryStatusCancelled: {},
	},
}

func (s DeliveryStatus) Valid() bool {
	switch s {
	case DeliveryStatusPlanned, DeliveryStatusReserved, DeliveryStatusQueued,
		DeliveryStatusSubmitting, DeliveryStatusProviderAccepted, DeliveryStatusConfirmed,
		DeliveryStatusSuppressed, DeliveryStatusDeferred, DeliveryStatusTransientFailed,
		DeliveryStatusTerminalFailed, DeliveryStatusUnknown, DeliveryStatusCancelled:
		return true
	default:
		return false
	}
}

func (s DeliveryStatus) CanTransitionTo(next DeliveryStatus) bool {
	if !s.Valid() || !next.Valid() || s == next {
		return false
	}
	_, allowed := deliveryStatusTransitions[s][next]
	return allowed
}

type DeliverySource string

const (
	DeliverySourceBroadcast  DeliverySource = "broadcast"
	DeliverySourceCampaign   DeliverySource = "campaign"
	DeliverySourceAutomation DeliverySource = "automation"
	DeliverySourceAPI        DeliverySource = "api"
	DeliverySourceLegacy     DeliverySource = "legacy"
)

type DeliveryEffectKeyInput struct {
	WorkspaceID   string
	SourceType    string
	SourceID      string
	SourceVersion string
	CustomerID    string
	NodeOrPhase   string
	Occurrence    string
	Variant       string
}

func canonicalDeliveryDimension(value string) string {
	return strings.TrimSpace(norm.NFKC.String(value))
}

func (i DeliveryEffectKeyInput) canonicalDimensions() ([]string, error) {
	dimensions := []struct {
		name  string
		value string
	}{
		{name: "workspace_id", value: i.WorkspaceID},
		{name: "source_type", value: strings.ToLower(i.SourceType)},
		{name: "source_id", value: i.SourceID},
		{name: "source_version", value: i.SourceVersion},
		{name: "customer_id", value: i.CustomerID},
		{name: "node_or_phase", value: i.NodeOrPhase},
		{name: "occurrence", value: i.Occurrence},
		{name: "variant", value: i.Variant},
	}
	values := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		value := canonicalDeliveryDimension(dimension.value)
		if value == "" {
			return nil, fmt.Errorf("%s is required", dimension.name)
		}
		values = append(values, value)
	}
	return values, nil
}

// EffectKey returns a deterministic business key. Occurrence is supplied by
// the caller as an event ID, snapshot ordinal, or another durable business
// sequence; this function deliberately never consults time or randomness.
func (i DeliveryEffectKeyInput) EffectKey() (string, error) {
	dimensions, err := i.canonicalDimensions()
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(dimensions)
	if err != nil {
		return "", fmt.Errorf("encode delivery effect key dimensions: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type DeliveryIntent struct {
	ID                string         `json:"id"`
	EffectKey         string         `json:"effect_key"`
	RequestHash       string         `json:"request_hash"`
	SourceType        DeliverySource `json:"source_type"`
	SourceID          string         `json:"source_id"`
	SourceVersion     string         `json:"source_version"`
	CustomerID        string         `json:"customer_id,omitempty"`
	LegacyIdentity    string         `json:"legacy_identity,omitempty"`
	Channel           string         `json:"channel"`
	TemplateID        string         `json:"template_id,omitempty"`
	TemplateVersion   int64          `json:"template_version,omitempty"`
	NodeOrPhase       string         `json:"node_or_phase"`
	Occurrence        string         `json:"occurrence"`
	Variant           string         `json:"variant"`
	Status            DeliveryStatus `json:"status"`
	SuppressionReason string         `json:"suppression_reason,omitempty"`
	Metadata          MapOfAny       `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type DeliveryAttempt struct {
	ID                string         `json:"id"`
	IntentID          string         `json:"intent_id"`
	AttemptNo         int            `json:"attempt_no"`
	Provider          string         `json:"provider"`
	RequestHash       string         `json:"request_hash"`
	ProviderMessageID string         `json:"provider_message_id,omitempty"`
	Status            DeliveryStatus `json:"status"`
	ClaimToken        string         `json:"claim_token,omitempty"`
	LeaseExpiresAt    *time.Time     `json:"lease_expires_at,omitempty"`
	SubmittedAt       *time.Time     `json:"submitted_at,omitempty"`
	AcceptedAt        *time.Time     `json:"accepted_at,omitempty"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	ErrorCategory     string         `json:"error_category,omitempty"`
	ErrorCode         string         `json:"error_code,omitempty"`
	ErrorDetail       string         `json:"error_detail,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type DeliveryReceiptLink struct {
	ID                string    `json:"id"`
	IntentID          string    `json:"intent_id"`
	AttemptID         string    `json:"attempt_id,omitempty"`
	Provider          string    `json:"provider"`
	ProviderMessageID string    `json:"provider_message_id,omitempty"`
	ReceiptID         string    `json:"receipt_id"`
	PayloadHash       string    `json:"payload_hash"`
	ReceivedAt        time.Time `json:"received_at"`
}

type DeliveryReconciliation struct {
	ID             string     `json:"id"`
	IntentID       string     `json:"intent_id"`
	AttemptID      string     `json:"attempt_id,omitempty"`
	Status         string     `json:"status"`
	Resolution     string     `json:"resolution,omitempty"`
	ActorID        string     `json:"actor_id,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	ProviderResult MapOfAny   `json:"provider_result,omitempty"`
	NextQueryAt    *time.Time `json:"next_query_at,omitempty"`
	LeaseToken     string     `json:"lease_token,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

var ErrDeliveryIntentHashConflict = errors.New("effect key already exists with a different delivery request")

type ReserveDeliveryResult struct {
	Intent       DeliveryIntent `json:"intent"`
	Created      bool           `json:"created"`
	QueueCreated bool           `json:"queue_created"`
}

type DeliveryRepository interface {
	ReserveIntent(context.Context, string, DeliveryIntent) (DeliveryIntent, bool, error)
	ReserveAndEnqueue(context.Context, string, DeliveryIntent, *EmailQueueEntry) (ReserveDeliveryResult, error)
	ResolveCustomerID(context.Context, string, string) (string, error)
	GetIntentByEffectKey(context.Context, string, string) (*DeliveryIntent, error)
	TransitionIntent(context.Context, string, string, DeliveryStatus, DeliveryStatus, time.Time) (bool, error)
}
