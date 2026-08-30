package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrJourneyIdentityUnresolved = errors.New("journey customer identity is unresolved")

type JourneyEntryOutcome string

const (
	JourneyEntryOutcomeEnrolled      JourneyEntryOutcome = "enrolled"
	JourneyEntryOutcomeAlreadyOnce   JourneyEntryOutcome = "already_once"
	JourneyEntryOutcomeReplayedEvent JourneyEntryOutcome = "replayed_event"
	JourneyEntryOutcomeGuardDeferred JourneyEntryOutcome = "guard_deferred"
	JourneyEntryOutcomeGuardDenied   JourneyEntryOutcome = "guard_denied"
)

func (o JourneyEntryOutcome) IsValid() bool {
	switch o {
	case JourneyEntryOutcomeEnrolled, JourneyEntryOutcomeAlreadyOnce,
		JourneyEntryOutcomeReplayedEvent, JourneyEntryOutcomeGuardDeferred,
		JourneyEntryOutcomeGuardDenied:
		return true
	default:
		return false
	}
}

type JourneyEntryGuard struct {
	Enabled       bool          `json:"enabled"`
	Cooldown      time.Duration `json:"cooldown,omitempty"`
	MaxConcurrent int           `json:"max_concurrent,omitempty"`
}

func (g JourneyEntryGuard) Validate() error {
	if !g.Enabled {
		return nil
	}
	if g.Cooldown < 0 || g.MaxConcurrent < 0 {
		return errors.New("journey entry cooldown and max_concurrent cannot be negative")
	}
	if g.Cooldown == 0 && g.MaxConcurrent == 0 {
		return errors.New("enabled journey entry guard requires cooldown or max_concurrent")
	}
	return nil
}

func JourneyEnrollmentDedupeKey(automationID, customerID string, frequency TriggerFrequency, originEventID string) (string, error) {
	if strings.TrimSpace(automationID) == "" || strings.TrimSpace(customerID) == "" {
		return "", errors.New("automation_id and customer_id are required")
	}
	input := ""
	switch frequency {
	case TriggerFrequencyOnce:
		input = automationID + "\x00" + customerID + "\x00once"
	case TriggerFrequencyEveryTime:
		if strings.TrimSpace(originEventID) == "" {
			return "", errors.New("every_time journey enrollment requires origin_event_id")
		}
		input = automationID + "\x00" + customerID + "\x00every_time\x00" + originEventID
	default:
		return "", errors.New("invalid journey enrollment frequency")
	}
	digest := sha256.Sum256([]byte(input))
	return hex.EncodeToString(digest[:]), nil
}

type JourneyEnrollment struct {
	ID                string           `json:"id"`
	AutomationID      string           `json:"automation_id"`
	AutomationVersion int              `json:"automation_version"`
	CustomerID        string           `json:"customer_id"`
	ContactEmail      string           `json:"contact_email,omitempty"`
	Frequency         TriggerFrequency `json:"frequency"`
	OriginEventID     string           `json:"origin_event_id,omitempty"`
	DedupeKey         string           `json:"dedupe_key"`
	EnteredAt         time.Time        `json:"entered_at"`
}

type JourneyInstance struct {
	ID                  string                  `json:"id"`
	EnrollmentID        string                  `json:"enrollment_id"`
	ContactAutomationID string                  `json:"contact_automation_id"`
	Status              ContactAutomationStatus `json:"status"`
	CurrentNodeID       string                  `json:"current_node_id,omitempty"`
	WaitingReason       string                  `json:"waiting_reason,omitempty"`
	NextScheduledAt     *time.Time              `json:"next_scheduled_at,omitempty"`
	StartedAt           time.Time               `json:"started_at"`
	CompletedAt         *time.Time              `json:"completed_at,omitempty"`
}

type JourneyEntryDecision struct {
	ID            string     `json:"id"`
	AutomationID  string     `json:"automation_id"`
	CustomerID    string     `json:"customer_id"`
	OriginEventID string     `json:"origin_event_id,omitempty"`
	Decision      string     `json:"decision"`
	Reason        string     `json:"reason,omitempty"`
	RetryAt       *time.Time `json:"retry_at,omitempty"`
	DecidedAt     time.Time  `json:"decided_at"`
}

type JourneyEnrollmentResult struct {
	Outcome             JourneyEntryOutcome `json:"outcome"`
	ContactAutomationID string              `json:"contact_automation_id,omitempty"`
	RetryAt             *time.Time          `json:"retry_at,omitempty"`
}

type JourneyPreflightSeverity string

const (
	JourneyPreflightBlocking JourneyPreflightSeverity = "blocking"
	JourneyPreflightWarning  JourneyPreflightSeverity = "warning"
)

type JourneyPreflightIssue struct {
	Code        string                   `json:"code"`
	Severity    JourneyPreflightSeverity `json:"severity"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	NodeID      string                   `json:"node_id,omitempty"`
	FixPath     string                   `json:"fix_path,omitempty"`
}

type JourneyTemplateCheck struct {
	NodeID          string `json:"node_id"`
	Channel         string `json:"channel"`
	TemplateID      string `json:"template_id"`
	TemplateVersion int64  `json:"template_version,omitempty"`
	Exists          bool   `json:"exists"`
	ChannelMatches  bool   `json:"channel_matches"`
	ProviderReady   bool   `json:"provider_ready"`
}

// JourneyPreflightSnapshot is bounded configuration metadata. It never
// contains Customer rows or audience members.
type JourneyPreflightSnapshot struct {
	Automation               *Automation            `json:"automation"`
	TemplateChecks           []JourneyTemplateCheck `json:"template_checks,omitempty"`
	VariableErrors           map[string][]string    `json:"variable_errors,omitempty"`
	HasFrequencyPolicy       bool                   `json:"has_frequency_policy"`
	MissingFrequencyChannels []string               `json:"missing_frequency_channels,omitempty"`
}

type JourneyPreflightRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
}

func (r JourneyPreflightRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.AutomationID) == "" {
		return errors.New("workspace_id and automation_id are required")
	}
	return nil
}

type JourneyPreflightResult struct {
	WorkspaceID   string                  `json:"workspace_id"`
	AutomationID  string                  `json:"automation_id"`
	Issues        []JourneyPreflightIssue `json:"issues"`
	BlockingCount int                     `json:"blocking_count"`
	WarningCount  int                     `json:"warning_count"`
	SummaryHash   string                  `json:"summary_hash"`
	GeneratedAt   time.Time               `json:"generated_at"`
	ExpiresAt     time.Time               `json:"expires_at"`
}

func (r *JourneyPreflightResult) Seal(automationUpdatedAt time.Time) error {
	payload, err := json.Marshal(struct {
		WorkspaceID  string                  `json:"workspace_id"`
		AutomationID string                  `json:"automation_id"`
		UpdatedAt    time.Time               `json:"updated_at"`
		Issues       []JourneyPreflightIssue `json:"issues"`
		ExpiresAt    time.Time               `json:"expires_at"`
	}{r.WorkspaceID, r.AutomationID, automationUpdatedAt.UTC(), r.Issues, r.ExpiresAt.UTC()})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	r.SummaryHash = hex.EncodeToString(digest[:]) + "." + strconv.FormatInt(r.ExpiresAt.Unix(), 10)
	return nil
}

type JourneyPreflightSource interface {
	LoadJourneyPreflightSnapshot(context.Context, string, string) (*JourneyPreflightSnapshot, error)
}

type JourneyPreflightEvaluator interface {
	PreflightAutomation(context.Context, JourneyPreflightRequest) (*JourneyPreflightResult, error)
	ValidateAutomationPreflight(context.Context, JourneyPreflightRequest, string, bool) error
}

var (
	ErrJourneyPreflightRequired            = errors.New("journey activation preflight is required")
	ErrJourneyPreflightChanged             = errors.New("journey activation preflight changed; run preflight again")
	ErrJourneyPreflightBlocked             = errors.New("journey activation preflight contains blocking issues")
	ErrJourneyPreflightWarningConfirmation = errors.New("journey activation warnings must be confirmed")
	ErrJourneyTraceNotFound                = errors.New("journey trace not found")
)

type JourneyCustomerLocator struct {
	CustomerID     string `json:"customer_id,omitempty"`
	CustomerNo     string `json:"customer_no,omitempty"`
	ExternalUserID string `json:"external_user_id,omitempty"`
	Email          string `json:"email,omitempty"`
}

func (l JourneyCustomerLocator) Validate() error {
	values := []string{l.CustomerID, l.CustomerNo, l.ExternalUserID, l.Email}
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	if count != 1 {
		return errors.New("exactly one of customer_id, customer_no, external_user_id, or email is required")
	}
	return nil
}

type JourneyInstanceListRequest struct {
	WorkspaceID  string                 `json:"workspace_id"`
	AutomationID string                 `json:"automation_id,omitempty"`
	Locator      JourneyCustomerLocator `json:"locator"`
	Status       string                 `json:"status,omitempty"`
	Limit        int                    `json:"limit,omitempty"`
	Offset       int                    `json:"offset,omitempty"`
}

func (r *JourneyInstanceListRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	if err := r.Locator.Validate(); err != nil {
		return err
	}
	if r.Limit == 0 {
		r.Limit = 50
	}
	if r.Limit < 1 || r.Limit > 200 || r.Offset < 0 {
		return errors.New("limit must be between 1 and 200 and offset cannot be negative")
	}
	if r.Status != "" && !ContactAutomationStatus(r.Status).IsValid() {
		return fmt.Errorf("invalid journey status: %s", r.Status)
	}
	return nil
}

type JourneyInstanceSummary struct {
	JourneyInstance
	AutomationID   string           `json:"automation_id"`
	AutomationName string           `json:"automation_name"`
	CustomerID     string           `json:"customer_id"`
	CustomerNo     string           `json:"customer_no"`
	ExternalUserID string           `json:"external_user_id,omitempty"`
	ContactEmail   string           `json:"contact_email,omitempty"`
	Frequency      TriggerFrequency `json:"frequency"`
	OriginEventID  string           `json:"origin_event_id,omitempty"`
	EntryDecision  string           `json:"entry_decision"`
	EntryReason    string           `json:"entry_reason,omitempty"`
}

type JourneyTraceRequest struct {
	WorkspaceID       string `json:"workspace_id"`
	JourneyInstanceID string `json:"journey_instance_id"`
}

func (r JourneyTraceRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.JourneyInstanceID) == "" {
		return errors.New("workspace_id and journey_instance_id are required")
	}
	return nil
}

type JourneyTraceEvent struct {
	ID         string                 `json:"id"`
	NodeID     string                 `json:"node_id,omitempty"`
	EventType  string                 `json:"event_type"`
	Status     string                 `json:"status"`
	Reason     string                 `json:"reason,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

type JourneyDeliveryLink struct {
	Intent   DeliveryIntent        `json:"intent"`
	Attempts []DeliveryAttempt     `json:"attempts,omitempty"`
	Receipts []DeliveryReceiptLink `json:"receipts,omitempty"`
}

type JourneyTrace struct {
	Instance   JourneyInstanceSummary `json:"instance"`
	Decisions  []JourneyEntryDecision `json:"entry_decisions"`
	Events     []JourneyTraceEvent    `json:"events"`
	Deliveries []JourneyDeliveryLink  `json:"deliveries"`
}

type JourneyTraceSource interface {
	ListJourneyInstances(context.Context, JourneyInstanceListRequest) ([]JourneyInstanceSummary, int, error)
	GetJourneyTrace(context.Context, JourneyTraceRequest) (*JourneyTrace, error)
}

type JourneyTraceReader interface {
	ListInstances(context.Context, JourneyInstanceListRequest) ([]JourneyInstanceSummary, int, error)
	GetTrace(context.Context, JourneyTraceRequest) (*JourneyTrace, error)
}
