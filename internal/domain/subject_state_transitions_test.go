package domain

import "testing"

func TestSubjectStateTransitionEventsOnlyOnRealChanges(t *testing.T) {
	old := []SubjectStateInterval{{StateKey: "member", Value: StateTrue, RuleRevision: 1}}
	next := []SubjectStateInterval{{SubjectType: "contract", SubjectID: "A", StateKey: "member", Value: StateFalse, RuleRevision: 1, ValidFrom: funnelTime(60)}}
	changes := SubjectStateTransitions(old, next, false)
	if len(changes) != 1 || changes[0].EventType != "state.exited" {
		t.Fatal(changes)
	}
	if len(SubjectStateTransitions(next, next, false)) != 0 {
		t.Fatal("recompute is not another state exit")
	}
	next[0].RuleRevision = 2
	if len(SubjectStateTransitions(old, next, false)) != 0 {
		t.Fatal("definition rebuild must not trigger historical marketing")
	}
	if len(SubjectStateTransitions(nil, next, false)) != 0 {
		t.Fatal("initial replay must not trigger")
	}
	next[0].Value = StateTrue
	if len(SubjectStateTransitions(nil, next, true)) != 1 {
		t.Fatal("live first satisfied state is a real entry")
	}
}
