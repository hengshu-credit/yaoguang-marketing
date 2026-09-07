package domain

import (
	"encoding/json"
	"time"
)

type ExecutionObservationSubject struct {
	CustomerID  string
	SubjectType string
	SubjectID   string
	Since       time.Time
}
type ExecutionObservationCounts struct {
	Occurrences int `json:"occurrences"`
	Customers   int `json:"customers"`
	Objects     int `json:"objects"`
}
type ExecutionObservationMetric struct {
	Key        string                     `json:"key"`
	Goal       bool                       `json:"goal"`
	Observed   ExecutionObservationCounts `json:"observed"`
	Attributed ExecutionObservationCounts `json:"attributed"`
	EventIDs   []string                   `json:"event_ids"`
}
type ExecutionObservations struct {
	ExecutionID     string                       `json:"execution_id"`
	Plan            *MarketingTrackingPlan       `json:"plan"`
	Watermark       time.Time                    `json:"watermark"`
	CohortCustomers int                          `json:"cohort_customers"`
	Events          []ExecutionObservationMetric `json:"events"`
	States          []ExecutionObservationMetric `json:"states"`
}

func ExecutionTrackingPlan(config json.RawMessage) (*MarketingTrackingPlan, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	paths := [][]string{{"settings", "metadata", "tracking_plan"}, {"settings", "tracking_plan"}, {"trigger_config", "tracking_plan"}, {"trigger", "tracking_plan"}, {"tracking_plan"}}
	for _, path := range paths {
		raw := config
		found := true
		for _, key := range path {
			var object map[string]json.RawMessage
			if json.Unmarshal(raw, &object) != nil {
				found = false
				break
			}
			raw = object[key]
			if len(raw) == 0 || string(raw) == "null" {
				found = false
				break
			}
		}
		if found {
			var p MarketingTrackingPlan
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			if err := p.Validate(); err != nil {
				return nil, err
			}
			return &p, nil
		}
	}
	return nil, nil
}
func SummarizeExecutionObservations(run string, p MarketingTrackingPlan, cohort []ExecutionObservationSubject, events []AnalysisEvent, states []SubjectStateInterval, links []AnalysisTouchpoint, watermark time.Time) ExecutionObservations {
	result := ExecutionObservations{ExecutionID: run, Plan: &p, Watermark: watermark, Events: []ExecutionObservationMetric{}, States: []ExecutionObservationMetric{}}
	customers := map[string]bool{}
	for _, c := range cohort {
		customers[c.CustomerID] = true
	}
	result.CohortCustomers = len(customers)
	type counter struct{ seen, customers, objects map[string]bool }
	newCounter := func() *counter { return &counter{map[string]bool{}, map[string]bool{}, map[string]bool{}} }
	add := func(c *counter, e AnalysisEvent) {
		c.seen[e.ID] = true
		if e.CustomerID != "" {
			c.customers[e.CustomerID] = true
		}
		object := e.SubjectType + ":" + e.SubjectID
		if p.SubjectType == "customer" {
			object = "customer:" + e.CustomerID
		}
		c.objects[object] = true
	}
	counts := func(c *counter) ExecutionObservationCounts {
		return ExecutionObservationCounts{len(c.seen), len(c.customers), len(c.objects)}
	}
	eligible := func(e AnalysisEvent) bool {
		if e.OccurredAt.After(watermark) {
			return false
		}
		for _, c := range cohort {
			if c.CustomerID != e.CustomerID || e.OccurredAt.Before(c.Since) || e.OccurredAt.After(c.Since.Add(time.Duration(p.WindowSeconds)*time.Second)) {
				continue
			}
			if p.SubjectType != "customer" && e.SubjectType != p.SubjectType {
				continue
			}
			if c.SubjectType != "" && c.SubjectType != "customer" && (e.SubjectType != c.SubjectType || e.SubjectID != c.SubjectID) {
				continue
			}
			return true
		}
		return false
	}
	keys := append(append([]string{}, p.Events...), p.GoalEvents...)
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		m := ExecutionObservationMetric{Key: key, EventIDs: []string{}}
		for _, goal := range p.GoalEvents {
			if goal == key {
				m.Goal = true
			}
		}
		observed, attributed := newCounter(), newCounter()
		for _, e := range events {
			if e.Code != key || !eligible(e) {
				continue
			}
			if !observed.seen[e.ID] {
				m.EventIDs = append(m.EventIDs, e.ID)
			}
			add(observed, e)
			if AttributeAnalysisEvent(e, links, p.WindowSeconds).RunID == run {
				add(attributed, e)
			}
		}
		m.Observed = counts(observed)
		m.Attributed = counts(attributed)
		result.Events = append(result.Events, m)
	}
	for _, key := range p.States {
		m := ExecutionObservationMetric{Key: key, EventIDs: []string{}}
		observed := newCounter()
		for _, state := range states {
			if state.StateKey != key || state.Value != StateTrue || state.ComputedAt.After(watermark) {
				continue
			}
			e := AnalysisEvent{ID: state.SubjectType + ":" + state.SubjectID + ":" + state.ValidFrom.Format(time.RFC3339Nano), CustomerID: state.CustomerID, SubjectType: state.SubjectType, SubjectID: state.SubjectID, OccurredAt: state.ValidFrom}
			for _, c := range cohort {
				if c.CustomerID == state.CustomerID && e.OccurredAt.Before(c.Since) && (state.ValidTo == nil || state.ValidTo.After(c.Since)) {
					e.OccurredAt = c.Since
					break
				}
			}
			if eligible(e) {
				add(observed, e)
			}
		}
		m.Observed = counts(observed)
		result.States = append(result.States, m)
	}
	return result
}
