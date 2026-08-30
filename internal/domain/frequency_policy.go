package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

type FrequencyPolicyScope string
type FrequencyWindowKind string
type FrequencyDenyAction string

const (
	FrequencyScopeCampaign        FrequencyPolicyScope = "campaign"
	FrequencyScopeTrigger         FrequencyPolicyScope = "trigger"
	FrequencyScopeWorkspaceGlobal FrequencyPolicyScope = "workspace_global"
	FrequencyWindowSliding        FrequencyWindowKind  = "sliding"
	FrequencyWindowCalendar       FrequencyWindowKind  = "calendar"
	FrequencyActionSuppress       FrequencyDenyAction  = "suppress"
	FrequencyActionDefer          FrequencyDenyAction  = "defer"
)

type FrequencyPolicy struct {
	ID            string               `json:"id"`
	Version       int                  `json:"version"`
	Name          string               `json:"name"`
	Scope         FrequencyPolicyScope `json:"scope"`
	ScopeRef      string               `json:"scope_ref,omitempty"`
	Channel       string               `json:"channel"`
	MaxEvents     int                  `json:"max_events"`
	WindowKind    FrequencyWindowKind  `json:"window_kind"`
	WindowSeconds int64                `json:"window_seconds"`
	Timezone      string               `json:"timezone,omitempty"`
	DenyAction    FrequencyDenyAction  `json:"deny_action"`
	Priority      int                  `json:"priority"`
	Enabled       bool                 `json:"enabled"`
	CreatedAt     time.Time            `json:"created_at"`
}

func (p FrequencyPolicy) Validate() error {
	if strings.TrimSpace(p.ID) == "" || p.Version <= 0 || strings.TrimSpace(p.Name) == "" {
		return errors.New("frequency policy id, version and name are required")
	}
	switch p.Scope {
	case FrequencyScopeCampaign, FrequencyScopeTrigger:
		if strings.TrimSpace(p.ScopeRef) == "" {
			return errors.New("campaign and trigger policies require scope_ref")
		}
	case FrequencyScopeWorkspaceGlobal:
		if p.ScopeRef != "" {
			return errors.New("workspace global policy must not have scope_ref")
		}
	default:
		return errors.New("frequency policy scope is invalid")
	}
	if strings.TrimSpace(p.Channel) == "" || p.MaxEvents <= 0 || p.WindowSeconds <= 0 {
		return errors.New("frequency policy channel, max_events and window are required")
	}
	switch p.WindowKind {
	case FrequencyWindowSliding:
	case FrequencyWindowCalendar:
		if strings.TrimSpace(p.Timezone) == "" {
			return errors.New("calendar frequency policy requires timezone")
		}
	default:
		return errors.New("frequency window kind is invalid")
	}
	if p.DenyAction != FrequencyActionSuppress && p.DenyAction != FrequencyActionDefer {
		return errors.New("frequency deny action is invalid")
	}
	return nil
}

type FrequencyDecision struct {
	ID            string               `json:"id"`
	ReservationID string               `json:"reservation_id"`
	EffectKey     string               `json:"effect_key"`
	CustomerID    string               `json:"customer_id"`
	Channel       string               `json:"channel"`
	Allowed       bool                 `json:"allowed"`
	Deferred      bool                 `json:"deferred"`
	MatchedScope  FrequencyPolicyScope `json:"matched_scope,omitempty"`
	PolicyIDs     []string             `json:"policy_ids"`
	Reason        string               `json:"reason,omitempty"`
	DecidedAt     time.Time            `json:"decided_at"`
}

type FrequencyPolicyRepository interface {
	SaveFrequencyPolicy(context.Context, string, FrequencyPolicy) error
	ResolveFrequencyPolicies(context.Context, string, string, string, string) ([]FrequencyPolicy, error)
	SaveFrequencyDecision(context.Context, string, FrequencyDecision) error
}
