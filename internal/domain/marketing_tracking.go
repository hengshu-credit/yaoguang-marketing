package domain

import "fmt"

// MarketingTrackingPlan is scoped to a task and never limits event collection.
type MarketingTrackingPlan struct {
	SubjectType   string   `json:"subject_type"`
	Events        []string `json:"events"`
	States        []string `json:"states"`
	GoalEvents    []string `json:"goal_events,omitempty"`
	WindowSeconds int64    `json:"window_seconds"`
}

func (p MarketingTrackingPlan) Validate() error {
	if p.SubjectType == "" {
		return fmt.Errorf("tracking subject_type is required")
	}
	if p.WindowSeconds <= 0 || p.WindowSeconds > 31536000 {
		return fmt.Errorf("tracking window must be between 1 second and 1 year")
	}
	for _, keys := range [][]string{p.Events, p.States, p.GoalEvents} {
		if len(keys) > 100 {
			return fmt.Errorf("at most 100 tracking items allowed")
		}
		seen := map[string]bool{}
		for _, key := range keys {
			if !businessKeyPattern.MatchString(key) || seen[key] {
				return fmt.Errorf("tracking keys must be valid and distinct")
			}
			seen[key] = true
		}
	}
	return nil
}

func (r MarketingEventRule) Validate() error {
	if r.SubjectType == "" || !businessKeyPattern.MatchString(r.SubjectType) {
		return fmt.Errorf("event rule subject_type is required")
	}
	if len(r.Events) == 0 || len(r.Events) > 100 {
		return fmt.Errorf("event rule requires 1 to 100 events")
	}
	seen := map[string]bool{}
	for _, code := range r.Events {
		if !businessKeyPattern.MatchString(code) || seen[code] {
			return fmt.Errorf("event rule codes must be valid and distinct")
		}
		seen[code] = true
	}
	if r.MaxCausationDepth < 0 || r.MaxCausationDepth > 32 {
		return fmt.Errorf("invalid causation depth limit")
	}
	if r.Condition != nil {
		return validateStateRule(*r.Condition, 0)
	}
	return nil
}
