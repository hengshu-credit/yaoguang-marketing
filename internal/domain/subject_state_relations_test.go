package domain

import "testing"

func TestSubjectStateRelatedEventWindowHistory(t *testing.T) {
	rule := StateRule{Kind: "object_any", SubjectType: "invoice", RelationKey: "contract_invoices", Children: []StateRule{{Kind: "event", Event: "paid", WindowSeconds: 10}}}
	defs := []SubjectStateDefinition{{Key: "recent_payment", SubjectType: "contract", Rule: rule}}
	ctx := StateEvaluationContext{At: funnelTime(30), Complete: true, Children: []StateObjectContext{{SubjectType: "invoice", SubjectID: "I", RelationKey: "contract_invoices", ValidFrom: ptrTime(funnelTime(0)), Events: []AnalysisEvent{funnelEvent("p", "I", "paid", 5)}}}}
	history, _, err := BuildSubjectStateTimeline(defs, ctx, nil, "contract", "C", "customer")
	if err != nil {
		t.Fatal(err)
	}
	if SubjectStateAt(history, "contract", "C", "recent_payment", funnelTime(7)) != StateTrue || SubjectStateAt(history, "contract", "C", "recent_payment", funnelTime(16)) != StateFalse {
		t.Fatal(history)
	}
}

func TestSubjectStateRelatedObjectRuleHonorsRelationInterval(t *testing.T) {
	rule := StateRule{Kind: "object_any", SubjectType: "invoice", RelationKey: "contract_invoices", Children: []StateRule{{Kind: "state", StateKey: "overdue", Value: StateTrue}}}
	ctx := StateEvaluationContext{At: funnelTime(30), Complete: true, Children: []StateObjectContext{{SubjectType: "invoice", SubjectID: "invoice-1", RelationKey: "contract_invoices", ValidFrom: ptrTime(funnelTime(10)), ValidTo: ptrTime(funnelTime(20)), States: map[string]StateValue{"overdue": StateTrue}}}}
	if EvaluateStateRule(rule, ctx).Value != StateFalse {
		t.Fatal("closed relationship should not contribute")
	}
	ctx.At = funnelTime(15)
	if EvaluateStateRule(rule, ctx).Value != StateTrue {
		t.Fatal("valid linked object ignored")
	}
}
