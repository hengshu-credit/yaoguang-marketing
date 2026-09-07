package domain

import "testing"

func TestSubjectStateIndependentUnknownAndWindows(t *testing.T) {
	defs := []SubjectStateDefinition{{Key: "verified", SubjectType: "customer", Rule: StateRule{Kind: "event", Event: "verified"}}, {Key: "member", SubjectType: "customer", Rule: StateRule{Kind: "event", Event: "member", WindowSeconds: 60}}}
	ctx := StateEvaluationContext{At: funnelTime(30), Events: []AnalysisEvent{funnelEvent("v", "A", "verified", 0), funnelEvent("m", "A", "member", 0)}, Complete: true}
	values, err := EvaluateSubjectStates(defs, ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if values["verified"].Value != StateTrue || values["member"].Value != StateTrue {
		t.Fatal(values)
	}
	ctx.At = funnelTime(60)
	values, err = EvaluateSubjectStates(defs, ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if values["member"].Value != StateFalse {
		t.Fatal("window must expire without new event")
	}
	values, err = EvaluateSubjectStates(defs, StateEvaluationContext{At: funnelTime(30)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if values["verified"].Value != StateUnknown {
		t.Fatal("missing history is unknown")
	}
}
func TestSubjectStateAggregateAndManualOverride(t *testing.T) {
	def := SubjectStateDefinition{Key: "has_overdue", SubjectType: "customer", Rule: StateRule{Kind: "object_any", SubjectType: "contract", Children: []StateRule{{Kind: "state", StateKey: "overdue", Value: StateTrue}}}}
	ctx := StateEvaluationContext{At: funnelTime(10), Complete: true, Children: []StateObjectContext{{SubjectType: "contract", States: map[string]StateValue{"overdue": StateFalse}}, {SubjectType: "contract", States: map[string]StateValue{"overdue": StateTrue}}}}
	values, err := EvaluateSubjectStates([]SubjectStateDefinition{def}, ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if values[def.Key].Value != StateTrue {
		t.Fatal("one paid object cannot clear another overdue object")
	}
	values, err = EvaluateSubjectStates([]SubjectStateDefinition{def}, ctx, []StateOverride{{StateKey: def.Key, Value: StateFalse, Reason: "Reviewed", ValidFrom: funnelTime(0), ValidTo: ptrTime(funnelTime(20))}})
	if err != nil {
		t.Fatal(err)
	}
	if values[def.Key].Value != StateFalse || values[def.Key].RuleValue != StateTrue || values[def.Key].Source != "manual" {
		t.Fatal(values)
	}
}
func TestSubjectStateExclusiveConflictAndDependencyCycle(t *testing.T) {
	defs := []SubjectStateDefinition{{Key: "a", SubjectType: "customer", Group: "stage", Exclusive: true, Rule: StateRule{Kind: "attribute", Field: "active", Operator: "eq", Operand: true}}, {Key: "b", SubjectType: "customer", Group: "stage", Exclusive: true, Rule: StateRule{Kind: "attribute", Field: "active", Operator: "eq", Operand: true}}}
	_, err := EvaluateSubjectStates(defs, StateEvaluationContext{At: funnelTime(1), Attributes: map[string]interface{}{"active": true}, Complete: true}, nil)
	if err == nil {
		t.Fatal("conflicting single-select states must fail")
	}
	defs[0].Rule = StateRule{Kind: "state", StateKey: "b", Value: StateTrue}
	defs[1].Rule = StateRule{Kind: "state", StateKey: "a", Value: StateTrue}
	if ValidateSubjectStateDefinitions(defs) == nil {
		t.Fatal("dependency cycle must fail")
	}
}

func TestSubjectStateCountsOccurrencesButNotTransportRetries(t *testing.T) {
	rule := StateRule{Kind: "event", Event: "login", Minimum: 2, WindowSeconds: 60}
	e := funnelEvent("one", "A", "login", 0)
	ctx := StateEvaluationContext{At: funnelTime(10), Complete: true, Events: []AnalysisEvent{e, e}}
	if EvaluateStateRule(rule, ctx).Value != StateFalse {
		t.Fatal("retry of one event cannot satisfy two occurrences")
	}
	e.ID = "two"
	ctx.Events = append(ctx.Events, e)
	if EvaluateStateRule(rule, ctx).Value != StateTrue {
		t.Fatal("different report IDs represent distinct repeated behavior")
	}
}

func TestSubjectStateHistoricalAggregateUsesChildIntervals(t *testing.T) {
	rule := StateRule{Kind: "object_any", SubjectType: "contract", Children: []StateRule{{Kind: "state", StateKey: "overdue", Value: StateTrue}}}
	ctx := StateEvaluationContext{At: funnelTime(15), Historical: true, Complete: true, Children: []StateObjectContext{{SubjectType: "contract", SubjectID: "A", States: map[string]StateValue{"overdue": StateFalse}}}, ChildHistory: []SubjectStateInterval{{SubjectType: "contract", SubjectID: "A", StateKey: "overdue", Value: StateTrue, ValidFrom: funnelTime(10), ValidTo: ptrTime(funnelTime(20))}, {SubjectType: "contract", SubjectID: "A", StateKey: "overdue", Value: StateFalse, ValidFrom: funnelTime(20)}}}
	if EvaluateStateRule(rule, ctx).Value != StateTrue {
		t.Fatal("historical child evidence must be used")
	}
	ctx.At = funnelTime(20)
	if EvaluateStateRule(rule, ctx).Value != StateFalse {
		t.Fatal("half-open historical child interval")
	}
}

func TestSubjectStateHistoricalAttributeUsesImmutableSnapshot(t *testing.T) {
	rule := StateRule{Kind: "attribute", Field: "tier", Operator: "eq", Operand: "gold"}
	ctx := StateEvaluationContext{At: funnelTime(15), Historical: true, Complete: true, Attributes: map[string]interface{}{"tier": "silver"}, AttributeHistory: []StateAttributeSnapshot{{ValidFrom: funnelTime(10), Attributes: map[string]interface{}{"tier": "gold"}}, {ValidFrom: funnelTime(20), Attributes: map[string]interface{}{"tier": "silver"}}}}
	if EvaluateStateRule(rule, ctx).Value != StateTrue {
		t.Fatal("immutable gold snapshot at event time ignored")
	}
	ctx.At = funnelTime(5)
	if EvaluateStateRule(rule, ctx).Value != StateUnknown {
		t.Fatal("before available evidence is unknown")
	}
}

func TestSubjectStateLateAttributeSnapshotCannotLeakFutureUnchangedFields(t *testing.T) {
	history := []StateAttributeSnapshot{{ValidFrom: funnelTime(10), Attributes: map[string]interface{}{"tier": "bronze"}}, {ValidFrom: funnelTime(20), Attributes: map[string]interface{}{"tier": "gold", "city": "future"}, ChangedKeys: []string{"tier"}}, {ValidFrom: funnelTime(30), Attributes: map[string]interface{}{"tier": "gold", "city": "future"}, ChangedKeys: []string{"city"}}}
	at20 := SubjectAttributesAt(history, funnelTime(20))
	if at20["tier"] != "gold" {
		t.Fatal(at20)
	}
	if _, exists := at20["city"]; exists {
		t.Fatal("late full snapshot leaked future field")
	}
}

func TestSubjectStateFirstHistoricalPatchDoesNotPretendFullCoverage(t *testing.T) {
	got := SubjectAttributesAt([]StateAttributeSnapshot{{ValidFrom: funnelTime(1), Attributes: map[string]interface{}{"tier": "gold", "future": true}, ChangedKeys: []string{"tier"}}}, funnelTime(2))
	if _, ok := got["future"]; ok {
		t.Fatal("first partial snapshot must remain partial")
	}
}
