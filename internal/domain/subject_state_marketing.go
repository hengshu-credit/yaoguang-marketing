package domain

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// MarketingEventRule is an execution-snapshot condition. Tracking selections are
// observations and never change the workspace ingestion catalogue.
type MarketingEventRule struct {
	Events            []string   `json:"events"`
	SubjectType       string     `json:"subject_type"`
	Condition         *StateRule `json:"condition,omitempty"`
	OncePerObject     bool       `json:"once_per_object,omitempty"`
	MaxCausationDepth int        `json:"max_causation_depth"`
	ExcludedSources   []string   `json:"excluded_sources,omitempty"`
}
type MarketingRuleContext struct {
	Subject         EventSubject
	Now             time.Time
	ProjectionReady bool
	Replay          bool
	CausationDepth  int
	Source          string
	AutomationID    string
	States          map[string]StateValue
	Evaluation      *StateEvaluationContext
}
type MarketingRuleDecision struct {
	Matched          bool   `json:"matched"`
	AllowSideEffects bool   `json:"allow_side_effects"`
	Reason           string `json:"reason"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
}

func EvaluateMarketingEvent(rule MarketingEventRule, event AnalysisEvent, ctx MarketingRuleContext) MarketingRuleDecision {
	result := MarketingRuleDecision{Reason: "event_not_selected"}
	for _, code := range rule.Events {
		if code == event.Code {
			result.Matched = true
			break
		}
	}
	if !result.Matched {
		return result
	}
	if rule.SubjectType != "" && rule.SubjectType != event.SubjectType {
		return MarketingRuleDecision{Reason: "subject_type_mismatch"}
	}
	if ctx.Subject.ID != "" && (ctx.Subject.ID != event.SubjectID || ctx.Subject.Type != event.SubjectType) {
		return MarketingRuleDecision{Reason: "waiting_for_same_object"}
	}
	if event.CustomerID == "" {
		return MarketingRuleDecision{Reason: "identity_pending"}
	}
	if !ctx.ProjectionReady {
		return MarketingRuleDecision{Reason: "state_projection_pending"}
	}
	evaluation := StateEvaluationContext{At: event.OccurredAt, Attributes: event.Properties, States: ctx.States, Complete: true}
	if ctx.Evaluation != nil {
		evaluation = *ctx.Evaluation
		evaluation.At = event.OccurredAt
		if evaluation.States == nil {
			evaluation.States = ctx.States
		}
	}
	if rule.Condition != nil && EvaluateStateRule(*rule.Condition, evaluation).Value != StateTrue {
		return MarketingRuleDecision{Reason: "condition_not_matched"}
	}
	result.Reason = "matched"
	if ctx.Replay || event.Backfill {
		result.Reason = "historical_replay"
		return result
	}
	if event.ExpiresAt != nil && !ctx.Now.Before(*event.ExpiresAt) {
		result.Reason = "event_expired"
		return result
	}
	maxDepth := rule.MaxCausationDepth
	if maxDepth < 1 {
		maxDepth = 8
	}
	if ctx.CausationDepth >= maxDepth {
		result.Reason = "causation_limit"
		return result
	}
	for _, source := range rule.ExcludedSources {
		if source == ctx.Source {
			result.Reason = "source_excluded"
			return result
		}
	}
	key := event.ID
	if rule.OncePerObject {
		key = event.SubjectType + ":" + event.SubjectID
	}
	result.IdempotencyKey = fmt.Sprintf("%x", sha256.Sum256([]byte(ctx.AutomationID+":"+key)))
	result.AllowSideEffects = true
	return result
}
