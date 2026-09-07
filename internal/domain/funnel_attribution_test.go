package domain

import "testing"

func TestFunnelAttributionUsesVerifiedTouchpointAndLastValidClick(t *testing.T) {
	event := funnelEvent("conversion", "A", "signed", 100)
	event.TrustedTouchpointID = "direct"
	links := []AnalysisTouchpoint{{ID: "direct", RunID: "run-a", CustomerID: "customer-1", Kind: "delivery", At: funnelTime(10)}, {ID: "later", RunID: "run-b", CustomerID: "customer-1", Kind: "click", At: funnelTime(90)}}
	a := AttributeAnalysisEvent(event, links, 3600)
	if a.RunID != "run-a" || a.Model != "direct" {
		t.Fatal(a)
	}
	event.TrustedTouchpointID = ""
	a = AttributeAnalysisEvent(event, links, 3600)
	if a.RunID != "run-b" || a.Model != "last_click" {
		t.Fatal(a)
	}
	event.CustomerID = "different"
	if AttributeAnalysisEvent(event, links, 3600).RunID != "" {
		t.Fatal("another customer cannot inherit attribution")
	}
	event.CustomerID = "customer-1"
	if AttributeAnalysisEvent(event, links, 5).RunID != "" {
		t.Fatal("attribution window must expire")
	}
}
func TestFunnelUnattributedEventsRemainInAllSiteScope(t *testing.T) {
	e := funnelEvent("a", "A", "start", 0)
	events := FilterAnalysisScope([]AnalysisEvent{e}, nil, "all_site", "", 3600)
	if len(events) != 1 {
		t.Fatal("unattributed events disappeared")
	}
	events = FilterAnalysisScope([]AnalysisEvent{e}, nil, "attributed", "run", 3600)
	if len(events) != 0 {
		t.Fatal("unattributed event credited to run")
	}
}
