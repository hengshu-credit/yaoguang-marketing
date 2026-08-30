package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

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
