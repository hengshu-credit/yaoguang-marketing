package domain

import "testing"

func TestSubjectStateTimelineExpiryAndOverrideHistory(t *testing.T) {
	defs := []SubjectStateDefinition{{Key: "member", SubjectType: "contract", Revision: 1, Rule: StateRule{Kind: "event", Event: "member", WindowSeconds: 60}}}
	ctx := StateEvaluationContext{At: funnelTime(100), Complete: true, Events: []AnalysisEvent{funnelEvent("m", "A", "member", 0)}}
	intervals, _, err := BuildSubjectStateTimeline(defs, ctx, []StateOverride{{StateKey: "member", Value: StateFalse, Reason: "review", ValidFrom: funnelTime(10), ValidTo: ptrTime(funnelTime(20))}}, "contract", "A", "c")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at    int
		value StateValue
	}{{5, StateTrue}, {10, StateFalse}, {20, StateTrue}, {60, StateFalse}} {
		if v := SubjectStateAt(intervals, "contract", "A", "member", funnelTime(tc.at)); v != tc.value {
			t.Fatalf("at %d: %s != %s", tc.at, v, tc.value)
		}
	}
	for i := 1; i < len(intervals); i++ {
		if intervals[i-1].ValidTo == nil || !intervals[i-1].ValidTo.Equal(intervals[i].ValidFrom) {
			t.Fatal("history intervals must be adjacent")
		}
	}
}

func TestSubjectStateObservedAttributesKeepPriorIntervals(t *testing.T) {
	defs := []SubjectStateDefinition{{Key: "active", SubjectType: "contract", Revision: 1, Rule: StateRule{Kind: "attribute", Field: "active", Operator: "eq", Operand: true}}}
	previous := []SubjectStateInterval{{SubjectType: "contract", SubjectID: "A", StateKey: "active", Value: StateTrue, ValidFrom: funnelTime(10), RuleRevision: 1, Source: "rule"}}
	current := []SubjectStateInterval{{SubjectType: "contract", SubjectID: "A", StateKey: "active", Value: StateFalse, ValidFrom: funnelTime(20), RuleRevision: 1, Source: "rule"}}
	merged := MergeObservedStateHistory(defs, previous, current, funnelTime(20))
	if SubjectStateAt(merged, "contract", "A", "active", funnelTime(15)) != StateTrue || SubjectStateAt(merged, "contract", "A", "active", funnelTime(20)) != StateFalse {
		t.Fatal(merged)
	}
}
