package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type BusinessConditionNodeConfig struct {
	Mode           string     `json:"mode"`
	SubjectType    string     `json:"subject_type"`
	Events         []string   `json:"events"`
	Condition      *StateRule `json:"condition,omitempty"`
	TimeoutSeconds int64      `json:"timeout_seconds"`
	OnTimeout      string     `json:"on_timeout"`
}

func (c BusinessConditionNodeConfig) Validate() error {
	if c.Mode != "wait" && c.Mode != "exit_if" && c.Mode != "continue_if" {
		return fmt.Errorf("business condition mode must be wait, exit_if or continue_if")
	}
	if len(c.Events) == 0 && c.Condition == nil {
		return fmt.Errorf("an event or state condition is required")
	}
	rule := MarketingEventRule{SubjectType: c.SubjectType, Events: c.Events, Condition: c.Condition}
	if len(rule.Events) == 0 {
		rule.Events = []string{"condition"}
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	if c.Mode == "wait" && (c.TimeoutSeconds < 1 || c.TimeoutSeconds > 31536000) {
		return fmt.Errorf("wait timeout must be between 1 and 31536000 seconds")
	}
	if c.Mode == "wait" && c.OnTimeout != "exit" && c.OnTimeout != "continue" {
		return fmt.Errorf("wait timeout action must be exit or continue")
	}
	return nil
}
func ParseBusinessCondition(config map[string]any) (BusinessConditionNodeConfig, error) {
	var c BusinessConditionNodeConfig
	raw, err := json.Marshal(config)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}

type JourneyBusinessConditionRepository interface {
	EvaluateJourneyBusinessCondition(context.Context, string, string, BusinessConditionNodeConfig, time.Time, time.Time) (bool, map[string]any, error)
}
