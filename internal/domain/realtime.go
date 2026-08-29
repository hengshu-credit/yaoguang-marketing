package domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type EventSubject struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	ContactEmail string `json:"contact_email,omitempty"`
}

type EventEnvelope struct {
	ID            uuid.UUID       `json:"id"`
	EventID       uuid.UUID       `json:"event_id"`
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	WorkspaceID   string          `json:"workspace_id"`
	Subject       EventSubject    `json:"subject"`
	Source        string          `json:"source"`
	OccurredAt    time.Time       `json:"occurred_at"`
	ReceivedAt    time.Time       `json:"received_at"`
	CorrelationID uuid.UUID       `json:"correlation_id"`
	CausationID   *uuid.UUID      `json:"causation_id,omitempty"`
	TraceID       string          `json:"trace_id,omitempty"`
	Data          json.RawMessage `json:"data"`
}

func (e EventEnvelope) Validate() error {
	if e.ID == uuid.Nil {
		return fmt.Errorf("id is required")
	}
	if e.EventID == uuid.Nil {
		return fmt.Errorf("event_id is required")
	}
	if e.Type == "" {
		return fmt.Errorf("type is required")
	}
	if e.SchemaVersion <= 0 {
		return fmt.Errorf("schema_version must be positive")
	}
	if e.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if e.Subject.Type == "" || e.Subject.ID == "" {
		return fmt.Errorf("subject type and id are required")
	}
	if e.Source == "" {
		return fmt.Errorf("source is required")
	}
	if e.OccurredAt.IsZero() || e.ReceivedAt.IsZero() {
		return fmt.Errorf("occurred_at and received_at are required")
	}
	if e.CorrelationID == uuid.Nil {
		return fmt.Errorf("correlation_id is required")
	}
	if len(e.Data) == 0 || !json.Valid(e.Data) {
		return fmt.Errorf("data must contain valid JSON")
	}
	return nil
}

type OutboxStatus string

const (
	OutboxStatusPending   OutboxStatus = "pending"
	OutboxStatusClaimed   OutboxStatus = "claimed"
	OutboxStatusPublished OutboxStatus = "published"
	OutboxStatusDead      OutboxStatus = "dead"
)

func (s OutboxStatus) IsValid() bool {
	switch s {
	case OutboxStatusPending, OutboxStatusClaimed, OutboxStatusPublished, OutboxStatusDead:
		return true
	default:
		return false
	}
}

type OutboxMessage struct {
	ID             uuid.UUID       `json:"id"`
	EventID        uuid.UUID       `json:"event_id"`
	Topic          string          `json:"topic"`
	RoutingKey     string          `json:"routing_key"`
	Payload        json.RawMessage `json:"payload"`
	Headers        json.RawMessage `json:"headers"`
	Status         OutboxStatus    `json:"status"`
	Attempts       int             `json:"attempts"`
	AvailableAt    time.Time       `json:"available_at"`
	ClaimedBy      *string         `json:"claimed_by,omitempty"`
	ClaimToken     *uuid.UUID      `json:"claim_token,omitempty"`
	ClaimExpiresAt *time.Time      `json:"claim_expires_at,omitempty"`
	PublishedAt    *time.Time      `json:"published_at,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type InboxStatus string

const (
	InboxStatusProcessing InboxStatus = "processing"
	InboxStatusCompleted  InboxStatus = "completed"
	InboxStatusFailed     InboxStatus = "failed"
)

func (s InboxStatus) IsValid() bool {
	switch s {
	case InboxStatusProcessing, InboxStatusCompleted, InboxStatusFailed:
		return true
	default:
		return false
	}
}

type InboxClaim struct {
	Consumer       string      `json:"consumer"`
	MessageID      uuid.UUID   `json:"message_id"`
	Status         InboxStatus `json:"status"`
	Attempts       int         `json:"attempts"`
	ClaimToken     uuid.UUID   `json:"claim_token"`
	ClaimExpiresAt time.Time   `json:"claim_expires_at"`
	ProcessedAt    *time.Time  `json:"processed_at,omitempty"`
	LastError      *string     `json:"last_error,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	Acquired       bool        `json:"-"`
}

type TriggerBinding struct {
	AutomationID      string          `json:"automation_id"`
	AutomationVersion int             `json:"automation_version"`
	EventType         string          `json:"event_type"`
	SubjectType       string          `json:"subject_type"`
	DependencyKeys    []string        `json:"dependency_keys"`
	ConditionHash     string          `json:"condition_hash"`
	CompiledCondition json.RawMessage `json:"compiled_condition"`
	CreatedAt         time.Time       `json:"created_at"`
}

type MatchEngine string

const (
	MatchEngineLegacy   MatchEngine = "legacy"
	MatchEngineRealtime MatchEngine = "realtime"
)

func (e MatchEngine) IsValid() bool {
	return e == MatchEngineLegacy || e == MatchEngineRealtime
}

type MatchAudit struct {
	EventID             uuid.UUID       `json:"event_id"`
	AutomationID        string          `json:"automation_id"`
	Engine              MatchEngine     `json:"engine"`
	Matched             bool            `json:"matched"`
	DecisionHash        string          `json:"decision_hash"`
	ContactAutomationID *string         `json:"contact_automation_id,omitempty"`
	Reason              json.RawMessage `json:"reason"`
	CreatedAt           time.Time       `json:"created_at"`
}

type SideEffectStatus string

const (
	SideEffectStatusReserved  SideEffectStatus = "reserved"
	SideEffectStatusSubmitted SideEffectStatus = "submitted"
	SideEffectStatusConfirmed SideEffectStatus = "confirmed"
	SideEffectStatusFailed    SideEffectStatus = "failed"
	SideEffectStatusUnknown   SideEffectStatus = "unknown"
)

func (s SideEffectStatus) IsValid() bool {
	switch s {
	case SideEffectStatusReserved, SideEffectStatusSubmitted, SideEffectStatusConfirmed,
		SideEffectStatusFailed, SideEffectStatusUnknown:
		return true
	default:
		return false
	}
}

type SideEffectExecution struct {
	EffectKey           string           `json:"effect_key"`
	ContactAutomationID string           `json:"contact_automation_id"`
	AutomationVersion   int              `json:"automation_version"`
	NodeID              string           `json:"node_id"`
	ExecutionVersion    int64            `json:"execution_version"`
	Channel             string           `json:"channel"`
	Status              SideEffectStatus `json:"status"`
	ProviderMessageID   *string          `json:"provider_message_id,omitempty"`
	RequestHash         string           `json:"request_hash"`
	Attempts            int              `json:"attempts"`
	LastError           *string          `json:"last_error,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type EventAppendResult struct {
	EventID    uuid.UUID `json:"event_id"`
	MessageID  uuid.UUID `json:"message_id"`
	ReceivedAt time.Time `json:"received_at"`
	Duplicate  bool      `json:"duplicate"`
}

var (
	ErrEventPayloadConflict   = errors.New("event id already exists with a different payload")
	ErrMatchAuditConflict     = errors.New("match audit decision changed for the same event and engine")
	ErrSideEffectHashConflict = errors.New("side effect key already exists with a different request hash")
)

// RealtimeRepository is the PostgreSQL authority boundary consumed by the
// relay and workers. Transactional inbox methods deliberately accept the
// caller's transaction so claiming and business state changes commit together.
type RealtimeRepository interface {
	AppendEvent(ctx context.Context, workspaceID string, envelope EventEnvelope, receivedAt time.Time) (EventAppendResult, error)
	ClaimOutbox(ctx context.Context, workspaceID, workerID string, now time.Time, lease time.Duration, limit int) ([]OutboxMessage, error)
	MarkOutboxPublished(ctx context.Context, workspaceID string, id, claimToken uuid.UUID, publishedAt time.Time) (bool, error)
	ReleaseOutbox(ctx context.Context, workspaceID string, id, claimToken uuid.UUID, availableAt time.Time, lastError string, dead bool) (bool, error)
	ClaimInbox(ctx context.Context, tx *sql.Tx, workspaceID, consumer string, messageID uuid.UUID, now time.Time, lease time.Duration) (InboxClaim, error)
	CompleteInbox(ctx context.Context, tx *sql.Tx, workspaceID, consumer string, messageID, claimToken uuid.UUID, completedAt time.Time) (bool, error)
	ListTriggerBindings(ctx context.Context, workspaceID, eventType, subjectType string) ([]TriggerBinding, error)
	WriteMatchAudit(ctx context.Context, workspaceID string, audit MatchAudit) error
	ReserveSideEffect(ctx context.Context, workspaceID string, execution SideEffectExecution) (SideEffectExecution, bool, error)
	GetEvent(ctx context.Context, workspaceID string, eventID uuid.UUID) (*EventEnvelope, error)
}

// WorkspaceCursorRepository hands relay replicas disjoint, ordered workspace
// scan windows while persisting the last assignment in the system database.
type WorkspaceCursorRepository interface {
	NextWorkspaceIDs(ctx context.Context, cursorName string, limit int) ([]string, error)
}

// CanonicalJSONHash normalizes JSON object ordering before hashing. The
// decoder preserves number spelling to avoid precision loss for external IDs.
func CanonicalJSONHash(payload json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode JSON payload: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return "", err
	}

	canonical, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode canonical JSON: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON payload contains multiple documents")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func BuildSideEffectKey(
	workspaceID string,
	contactAutomationID string,
	automationVersion int,
	nodeID string,
	executionVersion int64,
	channel string,
) string {
	hasher := sha256.New()
	writeHashPart(hasher, workspaceID)
	writeHashPart(hasher, contactAutomationID)
	writeHashPart(hasher, strconv.Itoa(automationVersion))
	writeHashPart(hasher, nodeID)
	writeHashPart(hasher, strconv.FormatInt(executionVersion, 10))
	writeHashPart(hasher, channel)
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeHashPart(hasher hash.Hash, value string) {
	_, _ = io.WriteString(hasher, strconv.Itoa(len(value)))
	_, _ = io.WriteString(hasher, ":")
	_, _ = io.WriteString(hasher, value)
}
