package domain

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// AnalysisEvent is the normalized ledger read model, never a second ingestion format.
type AnalysisEvent struct {
	ID                  string                 `json:"id"`
	CustomerID          string                 `json:"customer_id"`
	SubjectType         string                 `json:"subject_type"`
	SubjectID           string                 `json:"subject_id"`
	Code                string                 `json:"code"`
	OccurredAt          time.Time              `json:"occurred_at"`
	ReceivedAt          time.Time              `json:"received_at"`
	Sequence            *int64                 `json:"sequence,omitempty"`
	Properties          map[string]interface{} `json:"properties"`
	AmountMinor         int64                  `json:"amount_minor,omitempty"`
	Currency            string                 `json:"currency,omitempty"`
	TransactionID       string                 `json:"transaction_id,omitempty"`
	ExpiresAt           *time.Time             `json:"expires_at,omitempty"`
	Backfill            bool                   `json:"backfill,omitempty"`
	TrustedTouchpointID string                 `json:"trusted_touchpoint_id,omitempty"`
	Attribution         AnalysisAttribution    `json:"attribution,omitempty"`
}
type FunnelStep struct {
	SubjectType   string                `json:"subject_type,omitempty"`
	RelationKey   string                `json:"relation_key,omitempty"`
	ID            string                `json:"id"`
	Name          string                `json:"name,omitempty"`
	Events        []string              `json:"events,omitempty"`
	AllEvents     []string              `json:"all_events,omitempty"`
	Optional      bool                  `json:"optional,omitempty"`
	WindowSeconds int64                 `json:"window_seconds,omitempty"`
	Filter        *StateRule            `json:"filter,omitempty"`
	StateFilters  map[string]StateValue `json:"state_filters,omitempty"`
}
type FunnelDefinition struct {
	SubjectType              string       `json:"subject_type"`
	CustomerViaObject        bool         `json:"customer_via_object,omitempty"`
	WindowSeconds            int64        `json:"window_seconds"`
	LateGraceSeconds         int64        `json:"late_grace_seconds,omitempty"`
	AllowSameTime            bool         `json:"allow_same_time,omitempty"`
	Steps                    []FunnelStep `json:"steps"`
	Scope                    string       `json:"scope,omitempty"`
	RunID                    string       `json:"run_id,omitempty"`
	AttributionWindowSeconds int64        `json:"attribution_window_seconds,omitempty"`
}
type FunnelQuery struct {
	Relations      []AnalysisObjectRelation `json:"-"`
	WorkspaceID    string                   `json:"workspace_id"`
	Key            string                   `json:"key,omitempty"`
	Definition     *FunnelDefinition        `json:"definition,omitempty"`
	Start          time.Time                `json:"start"`
	End            time.Time                `json:"end"`
	Watermark      time.Time                `json:"watermark"`
	IncludeDetails bool                     `json:"include_details,omitempty"`
}
type FunnelStepResult struct {
	ID             string           `json:"id"`
	Objects        int              `json:"objects"`
	Occurrences    int              `json:"occurrences"`
	Pending        int              `json:"pending"`
	Expired        int              `json:"expired"`
	StepRate       *float64         `json:"step_rate"`
	CumulativeRate *float64         `json:"cumulative_rate"`
	AmountMinor    map[string]int64 `json:"amount_minor"`
	Branches       map[string]int   `json:"branches"`
}
type FunnelExplanation struct {
	BoundSubjects map[string]string   `json:"bound_subjects,omitempty"`
	SubjectType   string              `json:"subject_type"`
	SubjectID     string              `json:"subject_id"`
	CustomerID    string              `json:"customer_id"`
	Reached       int                 `json:"reached"`
	EventIDs      []string            `json:"event_ids"`
	StartedAt     time.Time           `json:"started_at"`
	LastStart     time.Time           `json:"last_start"`
	Deadline      time.Time           `json:"deadline"`
	Mature        bool                `json:"mature"`
	Attribution   AnalysisAttribution `json:"attribution,omitempty"`
}
type FunnelResult struct {
	Steps      []FunnelStepResult  `json:"steps"`
	Entered    int                 `json:"entered"`
	Converted  int                 `json:"converted"`
	Mature     int                 `json:"mature"`
	Pending    int                 `json:"pending"`
	Excluded   int                 `json:"excluded"`
	Watermark  time.Time           `json:"watermark"`
	ComputedAt time.Time           `json:"computed_at"`
	Details    []FunnelExplanation `json:"details,omitempty"`
}

func (d FunnelDefinition) Validate() error {
	if d.Scope != "" && d.Scope != "all_site" && d.Scope != "recipients" && d.Scope != "attributed" {
		return fmt.Errorf("unsupported analysis scope")
	}
	if (d.Scope == "recipients" || d.Scope == "attributed") && d.RunID == "" {
		return fmt.Errorf("run_id required for scoped analysis")
	}
	if d.AttributionWindowSeconds < 0 || d.AttributionWindowSeconds > 31536000 {
		return fmt.Errorf("invalid attribution window")
	}
	if d.SubjectType == "" {
		return fmt.Errorf("subject_type is required")
	}
	if d.WindowSeconds < 1 || d.WindowSeconds > 31536000 {
		return fmt.Errorf("window must be between 1 second and 1 year")
	}
	if d.LateGraceSeconds < 0 || d.LateGraceSeconds > 31536000 {
		return fmt.Errorf("invalid late grace")
	}
	if len(d.Steps) < 1 || len(d.Steps) > 30 {
		return fmt.Errorf("funnel requires 1 to 30 steps")
	}
	ids := map[string]bool{}
	boundTypes := map[string]bool{d.SubjectType: true}
	for i, s := range d.Steps {
		typ := s.SubjectType
		if typ == "" {
			typ = d.SubjectType
		}
		if i == 0 && typ != d.SubjectType {
			return fmt.Errorf("entry subject must match the cohort object")
		}
		if !boundTypes[typ] && s.RelationKey == "" {
			return fmt.Errorf("step %s requires an object relation", s.ID)
		}
		boundTypes[typ] = true
		if s.ID == "" || ids[s.ID] {
			return fmt.Errorf("step %d requires a unique ID", i+1)
		}
		ids[s.ID] = true
		if (len(s.Events) == 0) == (len(s.AllEvents) == 0) {
			return fmt.Errorf("step %s requires either OR events or parallel events", s.ID)
		}
		if i == 0 && (s.Optional || len(s.AllEvents) > 0) {
			return fmt.Errorf("entry must be a required event step")
		}
		if len(s.AllEvents) > 10 {
			return fmt.Errorf("parallel group supports at most 10 events")
		}
		codes := append(append([]string(nil), s.Events...), s.AllEvents...)
		uniqueCodes := map[string]bool{}
		for _, code := range codes {
			if code == "" || uniqueCodes[code] {
				return fmt.Errorf("step %s requires distinct nonempty event codes", s.ID)
			}
			uniqueCodes[code] = true
		}
		if s.WindowSeconds < 0 || s.WindowSeconds > d.WindowSeconds {
			return fmt.Errorf("invalid step window")
		}
		if s.Filter != nil {
			if err := validateStateRule(*s.Filter, 0); err != nil {
				return err
			}
		}
		for _, v := range s.StateFilters {
			if !v.Valid() {
				return fmt.Errorf("invalid state filter")
			}
		}
	}
	return nil
}

type funnelPath struct {
	bindings map[string]string
	stepIDs  []int
	last     int
	start    int
	ids      []int
	reached  int
}

func ComputeFunnel(d FunnelDefinition, q FunnelQuery, input []AnalysisEvent, history []SubjectStateInterval) (FunnelResult, error) {
	return ComputeFunnelContext(context.Background(), d, q, input, history)
}

func ComputeFunnelContext(ctx context.Context, d FunnelDefinition, q FunnelQuery, input []AnalysisEvent, history []SubjectStateInterval) (FunnelResult, error) {
	result := FunnelResult{Watermark: q.Watermark, ComputedAt: time.Now().UTC(), Steps: make([]FunnelStepResult, len(d.Steps))}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	workBudget := 5_000_000
	if err := d.Validate(); err != nil {
		return result, err
	}
	if q.Start.IsZero() || !q.End.After(q.Start) || q.Watermark.IsZero() {
		return result, fmt.Errorf("valid start, end and watermark required")
	}
	groups := map[string][]AnalysisEvent{}
	eligible := []AnalysisEvent{}
	seen := map[string]bool{}
	for _, e := range input {
		if seen[e.ID] || e.OccurredAt.After(q.Watermark) || e.ReceivedAt.After(q.Watermark) {
			continue
		}
		seen[e.ID] = true
		eligible = append(eligible, e)
		key := e.SubjectID
		if d.SubjectType == "customer" {
			key = e.CustomerID
		} else if e.SubjectType != d.SubjectType {
			continue
		}
		if key == "" {
			result.Excluded++
			continue
		}
		groups[key] = append(groups[key], e)
	}
	if d.HasLinkedSteps() {
		if err := expandFunnelGroups(d, groups, eligible, q.Relations); err != nil {
			return result, err
		}
	}
	counted := make([]map[string]bool, len(d.Steps))
	waiting := make([]map[string]bool, len(d.Steps))
	moneySeen := make([]map[string]bool, len(d.Steps))
	eventSeen := make([]map[string]bool, len(d.Steps))
	branchSeen := make([]map[string]map[string]bool, len(d.Steps))
	for i, s := range d.Steps {
		result.Steps[i] = FunnelStepResult{ID: s.ID, AmountMinor: map[string]int64{}, Branches: map[string]int{}}
		counted[i] = map[string]bool{}
		waiting[i] = map[string]bool{}
		moneySeen[i] = map[string]bool{}
		eventSeen[i] = map[string]bool{}
		branchSeen[i] = map[string]map[string]bool{}
	}
	matureKeys, pendingKeys := map[string]bool{}, map[string]bool{}
	for subject, events := range groups {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].OccurredAt.Equal(events[j].OccurredAt) {
				if events[i].Sequence != nil && events[j].Sequence != nil {
					return *events[i].Sequence < *events[j].Sequence
				}
				return events[i].ID < events[j].ID
			}
			return events[i].OccurredAt.Before(events[j].OccurredAt)
		})
		paths := []funnelPath{}
		var lastStart time.Time
		for i, e := range events {
			if (d.SubjectType == "customer" || e.SubjectType == d.SubjectType && e.SubjectID == subject) && !e.OccurredAt.Before(q.Start) && e.OccurredAt.Before(q.End) && funnelStepEvent(d.Steps[0], e, history) {
				paths = append(paths, funnelPath{last: i, start: i, ids: []int{i}, stepIDs: []int{i}, reached: 1, bindings: map[string]string{d.SubjectType: subject}})
				lastStart = e.OccurredAt
			}
		}
		if len(paths) == 0 {
			continue
		}
		countKey := subject
		if d.CustomerViaObject {
			countKey = events[paths[0].start].CustomerID
			if countKey == "" {
				result.Excluded++
				continue
			}
		}
		deadline := lastStart.Add(time.Duration(d.WindowSeconds+d.LateGraceSeconds) * time.Second)
		mature := !q.Watermark.Before(deadline)
		if mature {
			matureKeys[countKey] = true
		} else {
			pendingKeys[countKey] = true
		}
		best := paths[0]
		for stepIndex, step := range d.Steps {
			if stepIndex > 0 {
				next := []funnelPath{}
				for _, p := range paths {
					stepDeadline := events[p.start].OccurredAt.Add(time.Duration(d.WindowSeconds) * time.Second)
					if step.WindowSeconds > 0 {
						candidate := events[p.last].OccurredAt.Add(time.Duration(step.WindowSeconds) * time.Second)
						if candidate.Before(stepDeadline) {
							stepDeadline = candidate
						}
					}
					if q.Watermark.Before(stepDeadline.Add(time.Duration(d.LateGraceSeconds) * time.Second)) {
						waiting[stepIndex][countKey] = true
					}
					if step.Optional {
						np := p
						np.stepIDs = nil
						np.reached = stepIndex + 1
						next = append(next, np)
					}
					matches, err := matchFunnelStep(ctx, d, step, p, events, history, q.Relations, &workBudget)
					if err != nil {
						return result, err
					}
					for _, m := range matches {
						m.reached = stepIndex + 1
						next = append(next, m)
						if len(next) > 100000 {
							return result, fmt.Errorf("funnel candidate limit exceeded; narrow the query")
						}
					}
				}
				paths = dedupFunnelPaths(next)
				if len(paths) > 100000 {
					return result, fmt.Errorf("funnel candidate limit exceeded; narrow the query")
				}
			}
			if len(paths) == 0 {
				break
			}
			counted[stepIndex][countKey] = true
			for _, p := range paths {
				if p.reached > best.reached {
					best = p
				}
				for _, eventIndex := range p.stepIDs {
					last := events[eventIndex]
					if funnelStepEvent(step, last, history) {
						codes := []string{last.Code}
						if len(step.AllEvents) > 0 {
							codes = step.AllEvents
						}
						for _, code := range codes {
							if branchSeen[stepIndex][code] == nil {
								branchSeen[stepIndex][code] = map[string]bool{}
							}
							branchSeen[stepIndex][code][countKey] = true
						}
					}
					eventKey := last.ID
					if !eventSeen[stepIndex][eventKey] {
						eventSeen[stepIndex][eventKey] = true
						result.Steps[stepIndex].Occurrences++
					}
					moneyKey := last.Currency + ":" + last.TransactionID
					if last.TransactionID != "" && last.Currency != "" && !moneySeen[stepIndex][moneyKey] {
						moneySeen[stepIndex][moneyKey] = true
						result.Steps[stepIndex].AmountMinor[last.Currency] += last.AmountMinor
					}
				}
			}
		}
		ids := []string{}
		for _, i := range best.ids {
			ids = append(ids, events[i].ID)
		}
		result.Details = append(result.Details, FunnelExplanation{BoundSubjects: best.bindings, SubjectType: d.SubjectType, SubjectID: subject, CustomerID: events[best.start].CustomerID, Reached: best.reached, EventIDs: ids, StartedAt: events[best.start].OccurredAt, LastStart: lastStart, Deadline: deadline, Mature: mature, Attribution: events[best.last].Attribution})
	}
	for key := range pendingKeys {
		delete(matureKeys, key)
	}
	result.Mature = len(matureKeys)
	result.Pending = len(pendingKeys)
	for i := range result.Steps {
		result.Steps[i].Objects = len(counted[i])
		for code, subjects := range branchSeen[i] {
			result.Steps[i].Branches[code] = len(subjects)
		}
		if i > 0 {
			result.Steps[i].StepRate = funnelRate(len(counted[i]), len(counted[i-1]))
			for key := range counted[i-1] {
				if !counted[i][key] {
					if waiting[i][key] {
						result.Steps[i].Pending++
					} else {
						result.Steps[i].Expired++
					}
				}
			}
		}
	}
	result.Entered = result.Steps[0].Objects
	result.Converted = result.Steps[len(result.Steps)-1].Objects
	for i := range result.Steps {
		result.Steps[i].CumulativeRate = funnelRate(result.Steps[i].Objects, result.Entered)
	}
	sort.Slice(result.Details, func(i, j int) bool { return result.Details[i].SubjectID < result.Details[j].SubjectID })
	return result, nil
}
func funnelRate(a, b int) *float64 {
	if b == 0 {
		return nil
	}
	v := float64(a) / float64(b)
	return &v
}
func funnelAfter(d FunnelDefinition, a, b AnalysisEvent) bool {
	if b.OccurredAt.After(a.OccurredAt) {
		return true
	}
	return b.OccurredAt.Equal(a.OccurredAt) && (d.AllowSameTime || (a.Sequence != nil && b.Sequence != nil && *b.Sequence > *a.Sequence))
}
func funnelStepEvent(s FunnelStep, e AnalysisEvent, history []SubjectStateInterval) bool {
	found := false
	codes := s.Events
	if len(s.AllEvents) > 0 {
		codes = s.AllEvents
	}
	for _, code := range codes {
		if code == e.Code {
			found = true
		}
	}
	if !found {
		return false
	}
	if s.Filter != nil && EvaluateStateRule(*s.Filter, StateEvaluationContext{At: e.OccurredAt, Attributes: e.Properties, Complete: true}).Value != StateTrue {
		return false
	}
	for key, value := range s.StateFilters {
		if SubjectStateAt(history, e.SubjectType, e.SubjectID, key, e.OccurredAt) != value {
			return false
		}
	}
	return true
}
func matchFunnelStep(ctx context.Context, d FunnelDefinition, s FunnelStep, p funnelPath, events []AnalysisEvent, history []SubjectStateInterval, relations []AnalysisObjectRelation, workBudget *int) ([]funnelPath, error) {
	out := []funnelPath{}
	deadline := events[p.start].OccurredAt.Add(time.Duration(d.WindowSeconds) * time.Second)
	if s.WindowSeconds > 0 {
		stepEnd := events[p.last].OccurredAt.Add(time.Duration(s.WindowSeconds) * time.Second)
		if stepEnd.Before(deadline) {
			deadline = stepEnd
		}
	}
	parallelGroups := map[string]map[string]int{}
	for i := p.last + 1; i < len(events); i++ {
		*workBudget--
		if *workBudget < 0 {
			return nil, fmt.Errorf("funnel evaluation budget exceeded; narrow the query")
		}
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		e := events[i]
		if e.OccurredAt.After(deadline) {
			break
		}
		if !funnelAfter(d, events[p.last], e) || !funnelStepEvent(s, e, history) || !funnelEventInBinding(d, s, p, e, relations) {
			continue
		}
		ids := append([]int(nil), p.ids...)
		if len(s.AllEvents) > 0 {
			key := funnelSubjectKey(e.SubjectType, e.SubjectID)
			parallel := parallelGroups[key]
			if parallel == nil {
				parallel = map[string]int{}
				parallelGroups[key] = parallel
			}
			parallel[e.Code] = i
			if len(parallel) != len(s.AllEvents) {
				continue
			}
			for _, code := range s.AllEvents {
				ids = append(ids, parallel[code])
			}
		} else {
			ids = append(ids, i)
		}
		out = append(out, funnelPath{last: i, start: p.start, ids: ids, stepIDs: append([]int(nil), ids[len(p.ids):]...), bindings: bindFunnelEvent(d, s, p, e)})
	}
	return out, nil
}
func dedupFunnelPaths(paths []funnelPath) []funnelPath {
	seen := map[string]bool{}
	out := []funnelPath{}
	for _, p := range paths {
		k := fmt.Sprintf("%d:%d:%d:%s", p.start, p.last, len(p.stepIDs), funnelBindingKey(p.bindings))
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}
