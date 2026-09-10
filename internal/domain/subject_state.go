package domain

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// StateValue deliberately distinguishes absent evidence from a negative result.
type StateValue string

const (
	StateTrue    StateValue = "true"
	StateFalse   StateValue = "false"
	StateUnknown StateValue = "unknown"
)

func (v StateValue) Valid() bool { return v == StateTrue || v == StateFalse || v == StateUnknown }

type StateRule struct {
	Kind          string      `json:"kind"`
	Children      []StateRule `json:"children,omitempty"`
	Event         string      `json:"event,omitempty"`
	RevokeEvent   string      `json:"revoke_event,omitempty"`
	WindowSeconds int64       `json:"window_seconds,omitempty"`
	Minimum       int         `json:"minimum,omitempty"`
	Field         string      `json:"field,omitempty"`
	Operator      string      `json:"operator,omitempty"`
	Operand       interface{} `json:"operand,omitempty"`
	StateKey      string      `json:"state_key,omitempty"`
	Value         StateValue  `json:"value,omitempty"`
	SubjectType   string      `json:"subject_type,omitempty"`
	RelationKey   string      `json:"relation_key,omitempty"`
}
type SubjectStateDefinition struct {
	Key         string    `json:"key"`
	SubjectType string    `json:"subject_type"`
	Group       string    `json:"group,omitempty"`
	Exclusive   bool      `json:"exclusive,omitempty"`
	Priority    int       `json:"priority,omitempty"`
	Revision    int       `json:"revision"`
	Rule        StateRule `json:"rule"`
}
type SubjectStateInterval struct {
	SubjectType        string     `json:"subject_type"`
	SubjectID          string     `json:"subject_id"`
	CustomerID         string     `json:"customer_id,omitempty"`
	StateKey           string     `json:"state_key"`
	Value              StateValue `json:"value"`
	ValidFrom          time.Time  `json:"valid_from"`
	ValidTo            *time.Time `json:"valid_to,omitempty"`
	RuleRevision       int        `json:"rule_revision"`
	ProjectionRevision int64      `json:"projection_revision"`
	Source             string     `json:"source"`
	Reason             string     `json:"reason,omitempty"`
	EventIDs           []string   `json:"event_ids,omitempty"`
	ComputedAt         time.Time  `json:"computed_at"`
}
type StateOverride struct {
	StateKey  string     `json:"state_key"`
	Value     StateValue `json:"value"`
	Reason    string     `json:"reason"`
	Actor     string     `json:"actor"`
	ValidFrom time.Time  `json:"valid_from"`
	ValidTo   *time.Time `json:"valid_to,omitempty"`
}
type StateEvaluation struct {
	Value     StateValue `json:"value"`
	RuleValue StateValue `json:"rule_value"`
	Source    string     `json:"source"`
	NextCheck *time.Time `json:"next_check,omitempty"`
	EventIDs  []string   `json:"event_ids,omitempty"`
}
type StateObjectContext struct {
	SubjectType      string
	SubjectID        string
	Attributes       map[string]interface{}
	States           map[string]StateValue
	ValidFrom        *time.Time
	ValidTo          *time.Time
	RelationKey      string
	AttributeHistory []StateAttributeSnapshot
	Events           []AnalysisEvent
}
type StateAttributeSnapshot struct {
	ValidFrom   time.Time              `json:"valid_from"`
	RecordedAt  time.Time              `json:"recorded_at"`
	Attributes  map[string]interface{} `json:"attributes"`
	ChangedKeys []string               `json:"changed_keys"`
}
type StateEvaluationContext struct {
	Historical       bool
	At               time.Time
	Events           []AnalysisEvent
	Attributes       map[string]interface{}
	States           map[string]StateValue
	Children         []StateObjectContext
	ChildHistory     []SubjectStateInterval
	AttributeHistory []StateAttributeSnapshot
	Complete         bool
}

func ValidateSubjectStateDefinitions(defs []SubjectStateDefinition) error {
	byKey := map[string]SubjectStateDefinition{}
	for _, d := range defs {
		if d.Key == "" || d.SubjectType == "" {
			return fmt.Errorf("state key and subject_type are required")
		}
		if _, ok := byKey[d.Key]; ok {
			return fmt.Errorf("duplicate state %s", d.Key)
		}
		if err := validateStateRule(d.Rule, 0); err != nil {
			return fmt.Errorf("state %s: %w", d.Key, err)
		}
		byKey[d.Key] = d
	}
	seen := map[string]int{}
	var visit func(string) error
	visit = func(k string) error {
		if seen[k] == 1 {
			return fmt.Errorf("state dependency cycle at %s", k)
		}
		if seen[k] == 2 {
			return nil
		}
		seen[k] = 1
		var walk func(StateRule) error
		walk = func(r StateRule) error {
			if r.Kind == "state" {
				if _, ok := byKey[r.StateKey]; !ok {
					return fmt.Errorf("unknown state dependency %s", r.StateKey)
				}
				if err := visit(r.StateKey); err != nil {
					return err
				}
			}
			if r.Kind != "object_any" {
				for _, c := range r.Children {
					if err := walk(c); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := walk(byKey[k].Rule); err != nil {
			return err
		}
		seen[k] = 2
		return nil
	}
	for k := range byKey {
		if err := visit(k); err != nil {
			return err
		}
	}
	return nil
}
func validateStateRule(r StateRule, depth int) error {
	if depth > 20 {
		return fmt.Errorf("condition nesting exceeds 20")
	}
	if r.WindowSeconds < 0 || r.WindowSeconds > 315360000 {
		return fmt.Errorf("invalid rule window")
	}
	switch r.Kind {
	case "all", "any":
		if len(r.Children) == 0 {
			return fmt.Errorf("condition children required")
		}
	case "not", "object_any":
		if len(r.Children) != 1 {
			return fmt.Errorf("condition needs one child")
		}
		if r.Kind == "object_any" && r.SubjectType == "" {
			return fmt.Errorf("object type required")
		}
	case "event":
		if r.Event == "" {
			return fmt.Errorf("event required")
		}
	case "attribute":
		if r.Field == "" {
			return fmt.Errorf("field required")
		}
		switch r.Operator {
		case "eq", "neq", "gt", "gte", "lt", "lte", "exists":
		default:
			return fmt.Errorf("unsupported comparison")
		}
	case "state":
		if r.StateKey == "" || !r.Value.Valid() {
			return fmt.Errorf("state and valid value required")
		}
	default:
		return fmt.Errorf("unknown rule kind %s", r.Kind)
	}
	for _, c := range r.Children {
		if err := validateStateRule(c, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func EvaluateSubjectStates(defs []SubjectStateDefinition, ctx StateEvaluationContext, overrides []StateOverride) (map[string]StateEvaluation, error) {
	if err := ValidateSubjectStateDefinitions(defs); err != nil {
		return nil, err
	}
	results := map[string]StateEvaluation{}
	byKey := map[string]SubjectStateDefinition{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	states := map[string]StateValue{}
	for k, v := range ctx.States {
		states[k] = v
	}
	ctx.States = states
	var eval func(string)
	eval = func(k string) {
		if _, ok := results[k]; ok {
			return
		}
		d := byKey[k]
		var deps func(StateRule)
		deps = func(r StateRule) {
			if r.Kind == "state" {
				eval(r.StateKey)
			}
			if r.Kind != "object_any" {
				for _, c := range r.Children {
					deps(c)
				}
			}
		}
		deps(d.Rule)
		v := EvaluateStateRule(d.Rule, ctx)
		v.RuleValue = v.Value
		v.Source = "rule"
		for _, o := range overrides {
			if o.StateKey == k && !ctx.At.Before(o.ValidFrom) && (o.ValidTo == nil || ctx.At.Before(*o.ValidTo)) {
				v.Value = o.Value
				v.Source = "manual"
				v.NextCheck = earliestTime(v.NextCheck, o.ValidTo)
			}
		}
		results[k] = v
		states[k] = v.Value
	}
	for k := range byKey {
		eval(k)
	}
	groups := map[string]string{}
	for _, d := range defs {
		if !d.Exclusive || d.Group == "" || results[d.Key].Value != StateTrue {
			continue
		}
		if prev, ok := groups[d.Group]; ok {
			pd := byKey[prev]
			if d.Priority == pd.Priority {
				return nil, fmt.Errorf("exclusive state conflict in group %s", d.Group)
			}
			loser := d.Key
			if d.Priority > pd.Priority {
				loser = prev
				groups[d.Group] = d.Key
			}
			v := results[loser]
			v.Value = StateFalse
			results[loser] = v
		} else {
			groups[d.Group] = d.Key
		}
	}
	return results, nil
}
func earliestTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil || a.Before(*b) {
		return a
	}
	return b
}
func EvaluateStateRule(r StateRule, c StateEvaluationContext) StateEvaluation {
	result := StateEvaluation{Value: StateUnknown}
	if r.Kind == "attribute" && (c.Historical || len(c.AttributeHistory) > 0) {
		attributes := SubjectAttributesAt(c.AttributeHistory, c.At)
		if attributes == nil {
			return result
		}
		c.Attributes = attributes
	}
	switch r.Kind {
	case "all", "any", "not":
		vals := []StateEvaluation{}
		for _, child := range r.Children {
			v := EvaluateStateRule(child, c)
			vals = append(vals, v)
			result.NextCheck = earliestTime(result.NextCheck, v.NextCheck)
			result.EventIDs = append(result.EventIDs, v.EventIDs...)
		}
		if r.Kind == "not" {
			if len(vals) == 1 {
				if vals[0].Value == StateTrue {
					result.Value = StateFalse
				} else if vals[0].Value == StateFalse {
					result.Value = StateTrue
				}
			}
			break
		}
		result.Value = StateTrue
		if r.Kind == "any" {
			result.Value = StateFalse
		}
		unknown := false
		for _, v := range vals {
			if r.Kind == "all" && v.Value == StateFalse {
				result.Value = StateFalse
				return result
			}
			if r.Kind == "any" && v.Value == StateTrue {
				result.Value = StateTrue
				return result
			}
			unknown = unknown || v.Value == StateUnknown
		}
		if unknown {
			result.Value = StateUnknown
		}
	case "attribute":
		value, ok := StateField(c.Attributes, r.Field)
		if r.Operator == "exists" {
			if ok {
				result.Value = StateTrue
			} else if c.Complete {
				result.Value = StateFalse
			}
			break
		}
		if !ok {
			break
		}
		matched := false
		switch r.Operator {
		case "eq":
			matched = reflect.DeepEqual(value, r.Operand)
		case "neq":
			matched = !reflect.DeepEqual(value, r.Operand)
		case "gt", "gte", "lt", "lte":
			a, aok := stateNumber(value)
			b, bok := stateNumber(r.Operand)
			if !aok || !bok {
				return result
			}
			switch r.Operator {
			case "gt":
				matched = a > b
			case "gte":
				matched = a >= b
			case "lt":
				matched = a < b
			case "lte":
				matched = a <= b
			}
		}
		result.Value = StateFalse
		if matched {
			result.Value = StateTrue
		}
	case "state":
		if v, ok := c.States[r.StateKey]; ok {
			if v == StateUnknown && r.Value != StateUnknown {
				break
			}
			result.Value = StateFalse
			if v == r.Value {
				result.Value = StateTrue
			}
		}
	case "object_any":
		if c.Complete {
			result.Value = StateFalse
		}
		for _, child := range c.Children {
			if child.SubjectType != r.SubjectType {
				continue
			}
			if r.RelationKey != "" && child.RelationKey != r.RelationKey {
				continue
			}
			if child.ValidFrom != nil && child.ValidFrom.After(c.At) {
				result.NextCheck = earliestTime(result.NextCheck, child.ValidFrom)
			}
			if child.ValidTo != nil && child.ValidTo.After(c.At) {
				result.NextCheck = earliestTime(result.NextCheck, child.ValidTo)
			}
			if child.ValidFrom != nil && child.ValidFrom.After(c.At) || child.ValidTo != nil && !c.At.Before(*child.ValidTo) {
				continue
			}
			if c.Historical {
				if child.ValidFrom != nil && child.ValidFrom.After(c.At) {
					continue
				}
				child.States = map[string]StateValue{}
				child.Attributes = nil
				for _, h := range c.ChildHistory {
					if h.SubjectType == child.SubjectType && h.SubjectID == child.SubjectID && !c.At.Before(h.ValidFrom) && (h.ValidTo == nil || c.At.Before(*h.ValidTo)) {
						child.States[h.StateKey] = h.Value
					}
				}
			}
			v := EvaluateStateRule(r.Children[0], StateEvaluationContext{At: c.At, Attributes: child.Attributes, States: child.States, Complete: c.Complete, Historical: c.Historical, AttributeHistory: child.AttributeHistory, Events: child.Events})
			result.NextCheck = earliestTime(result.NextCheck, v.NextCheck)
			if v.Value == StateTrue {
				result.Value = StateTrue
				break
			}
			if v.Value == StateUnknown {
				result.Value = StateUnknown
			}
		}
	case "event":
		if c.Complete {
			result.Value = StateFalse
		}
		count := 0
		seenEvents := map[string]bool{}
		var revoke time.Time
		for _, e := range c.Events {
			if !e.OccurredAt.After(c.At) && e.Code == r.RevokeEvent && e.OccurredAt.After(revoke) {
				revoke = e.OccurredAt
			}
		}
		for _, e := range c.Events {
			if e.Code != r.Event || e.OccurredAt.After(c.At) || (!revoke.IsZero() && !e.OccurredAt.After(revoke)) {
				continue
			}
			if seenEvents[e.ID] {
				continue
			}
			seenEvents[e.ID] = true
			if r.WindowSeconds > 0 {
				expiry := e.OccurredAt.Add(time.Duration(r.WindowSeconds) * time.Second)
				if !c.At.Before(expiry) {
					continue
				}
				result.NextCheck = earliestTime(result.NextCheck, &expiry)
			}
			count++
			result.EventIDs = append(result.EventIDs, e.ID)
		}
		minimum := r.Minimum
		if minimum < 1 {
			minimum = 1
		}
		if count >= minimum {
			result.Value = StateTrue
		}
	}
	return result
}
func StateField(data map[string]interface{}, path string) (interface{}, bool) {
	if value, ok := data[strings.TrimPrefix(path, "$.")]; ok {
		return value, true
	}
	var v interface{} = data
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		v, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

// Replay only changed fields from late snapshots; unrelated fields in a full
// current profile must not leak backwards into the event's business time.
func SubjectAttributesAt(history []StateAttributeSnapshot, at time.Time) map[string]interface{} {
	ordered := append([]StateAttributeSnapshot(nil), history...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].ValidFrom.Equal(ordered[j].ValidFrom) {
			return ordered[i].RecordedAt.Before(ordered[j].RecordedAt)
		}
		return ordered[i].ValidFrom.Before(ordered[j].ValidFrom)
	})
	var attributes map[string]interface{}
	for _, h := range ordered {
		if h.ValidFrom.After(at) {
			continue
		}
		if attributes == nil {
			attributes = map[string]interface{}{}
		}
		if h.ChangedKeys == nil {
			attributes = map[string]interface{}{}
			for k, v := range h.Attributes {
				attributes[k] = v
			}
			continue
		}
		for _, k := range h.ChangedKeys {
			if v, ok := h.Attributes[k]; ok {
				attributes[k] = v
			} else {
				delete(attributes, k)
			}
		}
	}
	return attributes
}
func stateNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
func SubjectStateAt(history []SubjectStateInterval, subjectType, subjectID, key string, at time.Time) StateValue {
	v := StateUnknown
	var latest time.Time
	for _, h := range history {
		if h.SubjectType == subjectType && h.SubjectID == subjectID && h.StateKey == key && !at.Before(h.ValidFrom) && (h.ValidTo == nil || at.Before(*h.ValidTo)) && !h.ValidFrom.Before(latest) {
			v = h.Value
			latest = h.ValidFrom
		}
	}
	return v
}
