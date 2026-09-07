package domain

import (
	"context"
	"testing"
	"time"
)

func funnelTime(seconds int) time.Time {
	return time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
}
func funnelEvent(id, object, code string, seconds int) AnalysisEvent {
	return AnalysisEvent{ID: id, CustomerID: "customer-1", SubjectType: "contract", SubjectID: object, Code: code, OccurredAt: funnelTime(seconds), ReceivedAt: funnelTime(seconds)}
}
func basicFunnel() FunnelDefinition {
	return FunnelDefinition{SubjectType: "contract", WindowSeconds: 3600, Steps: []FunnelStep{{ID: "start", Events: []string{"start"}}, {ID: "finish", Events: []string{"finish"}}}}
}
func TestFunnelSameObjectAndMultipleStarts(t *testing.T) {
	result, err := ComputeFunnel(basicFunnel(), FunnelQuery{Start: funnelTime(0), End: funnelTime(8000), Watermark: funnelTime(10000)}, []AnalysisEvent{funnelEvent("a", "A", "start", 0), funnelEvent("b", "B", "finish", 10), funnelEvent("c", "C", "start", 0), funnelEvent("d", "C", "start", 7200), funnelEvent("e", "C", "finish", 7210)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps[0].Objects != 2 || result.Steps[1].Objects != 1 {
		t.Fatalf("cross-object join or earliest-only bug: %+v", result)
	}
	if result.Pending != 1 || result.Mature != 1 {
		t.Fatalf("last eligible start controls maturity: %+v", result)
	}
}
func TestFunnelParallelOptionalORAndHistoricalState(t *testing.T) {
	def := basicFunnel()
	def.Steps = []FunnelStep{{ID: "start", Events: []string{"start"}}, {ID: "documents", AllEvents: []string{"front", "back"}}, {ID: "optional", Events: []string{"optional"}, Optional: true}, {ID: "finish", Events: []string{"finish", "approved"}, StateFilters: map[string]StateValue{"eligible": StateTrue}}}
	events := []AnalysisEvent{funnelEvent("a", "A", "start", 0), funnelEvent("b", "A", "back", 10), funnelEvent("c", "A", "front", 20), funnelEvent("d", "A", "approved", 30)}
	history := []SubjectStateInterval{{SubjectType: "contract", SubjectID: "A", StateKey: "eligible", Value: StateTrue, ValidFrom: funnelTime(0), ValidTo: ptrTime(funnelTime(31))}, {SubjectType: "contract", SubjectID: "A", StateKey: "eligible", Value: StateFalse, ValidFrom: funnelTime(31)}}
	result, err := ComputeFunnel(def, FunnelQuery{Start: funnelTime(0), End: funnelTime(100), Watermark: funnelTime(4000)}, events, history)
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps[3].Objects != 1 {
		t.Fatalf("expected parallel and historical state match: %+v", result)
	}
}
func TestFunnelStrictTimestampAndDedupCurrency(t *testing.T) {
	def := basicFunnel()
	events := []AnalysisEvent{funnelEvent("a", "A", "start", 0), funnelEvent("b", "A", "finish", 0)}
	result, err := ComputeFunnel(def, FunnelQuery{Start: funnelTime(0), End: funnelTime(100), Watermark: funnelTime(4000)}, events, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps[1].Objects != 0 {
		t.Fatal("equal timestamps do not establish sequence")
	}
	def.AllowSameTime = true
	events[1].AmountMinor = 1250
	events[1].Currency = "CNY"
	events[1].TransactionID = "payment-1"
	events = append(events, events[1])
	result, err = ComputeFunnel(def, FunnelQuery{Start: funnelTime(0), End: funnelTime(100), Watermark: funnelTime(4000)}, events, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps[1].AmountMinor["CNY"] != 1250 {
		t.Fatalf("duplicate money: %+v", result)
	}
}
func TestFunnelEmptyRatesAndEntryBoundary(t *testing.T) {
	r, e := ComputeFunnel(basicFunnel(), FunnelQuery{Start: funnelTime(0), End: funnelTime(10), Watermark: funnelTime(20)}, []AnalysisEvent{funnelEvent("a", "A", "start", 10)}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if r.Steps[1].CumulativeRate != nil {
		t.Fatal("zero denominator must be null")
	}
}
func ptrTime(t time.Time) *time.Time { return &t }

func TestFunnelCancelledQueryStopsEvaluation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ComputeFunnelContext(ctx, basicFunnel(), FunnelQuery{Start: funnelTime(0), End: funnelTime(10), Watermark: funnelTime(20)}, []AnalysisEvent{funnelEvent("a", "A", "start", 0)}, nil)
	if err != context.Canceled {
		t.Fatalf("cancel ignored: %v", err)
	}
}
