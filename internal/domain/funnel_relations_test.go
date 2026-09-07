package domain

import "testing"

func TestFunnelCustomerStepDoesNotReplaceBoundSiblingObject(t *testing.T) {
	d := FunnelDefinition{SubjectType: "customer", WindowSeconds: 60, Steps: []FunnelStep{{ID: "click", Events: []string{"clicked"}}, {ID: "apply", SubjectType: "application", RelationKey: "customer_applications", Events: []string{"applied"}}, {ID: "draw", SubjectType: "order", RelationKey: "application_orders", Events: []string{"drawn"}}, {ID: "reminder", SubjectType: "customer", Events: []string{"reminded"}}, {ID: "pay", SubjectType: "order", Events: []string{"paid"}}}}
	event := func(id, typ, subject, code string, at int) AnalysisEvent {
		e := funnelEvent(id, subject, code, at)
		e.SubjectType = typ
		return e
	}
	q := FunnelQuery{Start: funnelTime(0), End: funnelTime(1), Watermark: funnelTime(20), Relations: []AnalysisObjectRelation{{Key: "customer_applications", FromType: "customer", FromID: "customer-1", ToType: "application", ToID: "A", ValidFrom: funnelTime(0)}, {Key: "application_orders", FromType: "application", FromID: "A", ToType: "order", ToID: "O1", ValidFrom: funnelTime(0)}, {Key: "application_orders", FromType: "application", FromID: "A", ToType: "order", ToID: "O2", ValidFrom: funnelTime(0)}}}
	events := []AnalysisEvent{event("c", "customer", "customer-1", "clicked", 0), event("a", "application", "A", "applied", 1), event("d", "order", "O1", "drawn", 2), event("r", "order", "O2", "reminded", 3), event("p", "order", "O2", "paid", 4)}
	result, err := ComputeFunnel(d, q, events, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 0 || result.Details[0].BoundSubjects["order"] != "O1" {
		t.Fatalf("customer step replaced bound order: %+v", result)
	}
	events = append(events, event("p1", "order", "O1", "paid", 5))
	result, err = ComputeFunnel(d, q, events, nil)
	if err != nil || result.Converted != 1 {
		t.Fatal(result, err)
	}
}

func TestFunnelCustomerOwnershipAnchorsApplicationAndOrder(t *testing.T) {
	d := FunnelDefinition{SubjectType: "customer", WindowSeconds: 60, Steps: []FunnelStep{{ID: "click", Events: []string{"clicked"}}, {ID: "apply", SubjectType: "application", RelationKey: "customer_applications", Events: []string{"applied"}}, {ID: "draw", SubjectType: "order", RelationKey: "application_orders", Events: []string{"drawn"}}, {ID: "pay", SubjectType: "order", Events: []string{"paid"}}}}
	e := func(id, typ, subject, code string, at int) AnalysisEvent {
		v := funnelEvent(id, subject, code, at)
		v.SubjectType = typ
		return v
	}
	events := []AnalysisEvent{e("c", "customer", "customer-1", "clicked", 0), e("a", "application", "A", "applied", 1), e("b", "application", "B", "applied", 2), e("d", "order", "A1", "drawn", 3), e("p", "order", "B1", "paid", 4)}
	q := FunnelQuery{Start: funnelTime(0), End: funnelTime(1), Watermark: funnelTime(20), Relations: []AnalysisObjectRelation{{Key: "customer_applications", FromType: "customer", FromID: "customer-1", ToType: "application", ToID: "A", ValidFrom: funnelTime(0)}, {Key: "customer_applications", FromType: "customer", FromID: "customer-1", ToType: "application", ToID: "B", ValidFrom: funnelTime(0)}, {Key: "application_orders", FromType: "application", FromID: "A", ToType: "order", ToID: "A1", ValidFrom: funnelTime(0)}, {Key: "application_orders", FromType: "application", FromID: "B", ToType: "order", ToID: "B1", ValidFrom: funnelTime(0)}}}
	r, err := ComputeFunnel(d, q, events, nil)
	if err != nil || r.Converted != 0 {
		t.Fatal(r, err)
	}
	events = append(events, e("p2", "order", "A1", "paid", 5))
	r, err = ComputeFunnel(d, q, events, nil)
	if err != nil || r.Converted != 1 || r.Steps[1].Occurrences != 2 {
		t.Fatal(r, err)
	}
}

func TestFunnelLinkedObjectsNeverMixParentsOrSiblings(t *testing.T) {
	d := FunnelDefinition{SubjectType: "application", WindowSeconds: 3600, Steps: []FunnelStep{{ID: "apply", Events: []string{"applied"}}, {ID: "draw", SubjectType: "order", RelationKey: "application_orders", Events: []string{"drawn"}}, {ID: "paid", SubjectType: "order", Events: []string{"paid"}}}}
	event := func(id, typ, subject, code string, at int) AnalysisEvent {
		e := funnelEvent(id, subject, code, at)
		e.SubjectType = typ
		return e
	}
	q := FunnelQuery{Start: funnelTime(0), End: funnelTime(1), Watermark: funnelTime(100), Relations: []AnalysisObjectRelation{{Key: "application_orders", FromType: "application", FromID: "A", ToType: "order", ToID: "A1", ValidFrom: funnelTime(0)}, {Key: "application_orders", FromType: "application", FromID: "A", ToType: "order", ToID: "A2", ValidFrom: funnelTime(0)}, {Key: "application_orders", FromType: "application", FromID: "B", ToType: "order", ToID: "B1", ValidFrom: funnelTime(0)}}}
	events := []AnalysisEvent{event("s", "application", "A", "applied", 0), event("x", "order", "B1", "drawn", 2), event("y", "order", "B1", "paid", 3), event("a", "order", "A1", "drawn", 4), event("b", "order", "A2", "paid", 5)}
	r, err := ComputeFunnel(d, q, events, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Converted != 0 || r.Steps[1].Objects != 1 {
		t.Fatal(r)
	}
	events = append(events, event("c", "order", "A1", "paid", 6))
	r, err = ComputeFunnel(d, q, events, nil)
	if err != nil || r.Converted != 1 {
		t.Fatal(r, err)
	}
	if r.Details[0].BoundSubjects["order"] != "A1" {
		t.Fatal(r.Details)
	}
}
