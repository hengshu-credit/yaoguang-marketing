package domain

import (
	"context"
	"sort"
	"time"
)

type FunnelRepository interface {
	Events(context.Context, string, string, time.Time, time.Time, time.Time) ([]AnalysisEvent, error)
	StateHistory(context.Context, string, time.Time, time.Time) ([]SubjectStateInterval, error)
}
type SubjectStateRepository interface {
	List(context.Context, string, string, string, bool) ([]SubjectStateInterval, error)
	Overrides(context.Context, string, string, string) ([]StateOverride, error)
	SetOverride(context.Context, string, string, string, StateOverride) error
	ReplaceProjection(context.Context, string, string, string, []SubjectStateInterval, *time.Time) error
	Due(context.Context, string, time.Time, int) ([]EventSubject, error)
	Context(context.Context, string, string, string) (StateEvaluationContext, error)
}

// Rebuilds one immutable projection at every event, window and override boundary.
// Source revisions remain in storage; a late event does not overwrite old evidence.
func BuildSubjectStateTimeline(defs []SubjectStateDefinition, ctx StateEvaluationContext, overrides []StateOverride, subjectType, subjectID, customerID string) ([]SubjectStateInterval, *time.Time, error) {
	boundaries := map[time.Time]bool{}
	var addRuleWindows func(StateRule, time.Time)
	addRuleWindows = func(r StateRule, at time.Time) {
		if r.WindowSeconds > 0 {
			boundaries[at.Add(time.Duration(r.WindowSeconds)*time.Second)] = true
		}
		for _, c := range r.Children {
			addRuleWindows(c, at)
		}
	}
	for _, e := range ctx.Events {
		boundaries[e.OccurredAt] = true
		for _, d := range defs {
			addRuleWindows(d.Rule, e.OccurredAt)
		}
	}
	for _, h := range ctx.ChildHistory {
		boundaries[h.ValidFrom] = true
		if h.ValidTo != nil {
			boundaries[*h.ValidTo] = true
		}
	}
	for _, child := range ctx.Children {
		for _, event := range child.Events {
			boundaries[event.OccurredAt] = true
			for _, definition := range defs {
				addRuleWindows(definition.Rule, event.OccurredAt)
			}
		}
		if child.ValidFrom != nil {
			boundaries[*child.ValidFrom] = true
		}
		if child.ValidTo != nil {
			boundaries[*child.ValidTo] = true
		}
		for _, h := range child.AttributeHistory {
			boundaries[h.ValidFrom] = true
		}
	}
	for _, h := range ctx.AttributeHistory {
		boundaries[h.ValidFrom] = true
	}
	for _, o := range overrides {
		boundaries[o.ValidFrom] = true
		if o.ValidTo != nil {
			boundaries[*o.ValidTo] = true
		}
	}
	boundaries[ctx.At] = true
	times := []time.Time{}
	for at := range boundaries {
		if !at.After(ctx.At) {
			times = append(times, at)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	out := []SubjectStateInterval{}
	last := map[string]int{}
	var nextCheck *time.Time
	for _, at := range times {
		point := ctx
		point.At = at
		// A current profile/child projection is not evidence of past attributes.
		// Only event-derived rules can be replayed before this observation.
		if at.Before(ctx.At) {
			point.Attributes = nil
			point.Historical = true
		}
		values, err := EvaluateSubjectStates(defs, point, overrides)
		if err != nil {
			return nil, nil, err
		}
		for _, d := range defs {
			v := values[d.Key]
			if at.Equal(ctx.At) {
				nextCheck = earliestTime(nextCheck, v.NextCheck)
			}
			if i, ok := last[d.Key]; ok {
				if out[i].Value == v.Value && out[i].Source == v.Source {
					continue
				}
				end := at
				out[i].ValidTo = &end
			}
			reason := ""
			if v.Source == "manual" {
				for _, o := range overrides {
					if o.StateKey == d.Key && !at.Before(o.ValidFrom) && (o.ValidTo == nil || at.Before(*o.ValidTo)) {
						reason = o.Reason
					}
				}
			}
			last[d.Key] = len(out)
			out = append(out, SubjectStateInterval{SubjectType: subjectType, SubjectID: subjectID, CustomerID: customerID, StateKey: d.Key, Value: v.Value, ValidFrom: at, RuleRevision: d.Revision, Source: v.Source, Reason: reason, EventIDs: v.EventIDs, ComputedAt: ctx.At})
		}
	}
	return out, nextCheck, nil
}

// Profile and related-object rules are observations, so retain prior observations
// instead of reinterpreting the past with today's mutable profile.
func MergeObservedStateHistory(defs []SubjectStateDefinition, previous, current []SubjectStateInterval, at time.Time) []SubjectStateInterval {
	dynamic := map[string]bool{}
	var depends func(StateRule) bool
	depends = func(r StateRule) bool {
		if r.Kind == "attribute" || r.Kind == "state" && dynamic[r.StateKey] {
			return true
		}
		for _, c := range r.Children {
			if depends(c) {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(defs); i++ {
		for _, d := range defs {
			if depends(d.Rule) {
				dynamic[d.Key] = true
			}
		}
	}
	out := []SubjectStateInterval{}
	for _, h := range current {
		if !dynamic[h.StateKey] {
			out = append(out, h)
		}
	}
	for key := range dynamic {
		var latest *SubjectStateInterval
		for _, h := range current {
			if h.StateKey == key && h.ValidTo == nil {
				copy := h
				latest = &copy
			}
		}
		if latest == nil {
			continue
		}
		latest.ValidFrom = at
		merged := false
		for _, h := range previous {
			if h.StateKey != key || !h.ValidFrom.Before(at) {
				continue
			}
			if h.ValidTo == nil || h.ValidTo.After(at) {
				if h.Value == latest.Value && h.Source == latest.Source && h.Reason == latest.Reason && h.RuleRevision == latest.RuleRevision {
					h.ValidTo = nil
					merged = true
				} else {
					end := at
					h.ValidTo = &end
				}
			}
			out = append(out, h)
		}
		if !merged {
			out = append(out, *latest)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StateKey == out[j].StateKey {
			return out[i].ValidFrom.Before(out[j].ValidFrom)
		}
		return out[i].StateKey < out[j].StateKey
	})
	return out
}
