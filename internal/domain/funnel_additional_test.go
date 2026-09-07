package domain

import (
	"testing"
	"time"
)

func TestFunnelOptionalSkipDoesNotInventOccurrencesOrMoney(t *testing.T) {
	d := basicFunnel()
	d.Steps = []FunnelStep{{ID: "entry", Events: []string{"start"}}, {ID: "optional", Events: []string{"upsell"}, Optional: true}, {ID: "finish", Events: []string{"finish"}}}
	entry := funnelEvent("s", "A", "start", 0)
	entry.AmountMinor = 100
	entry.Currency = "CNY"
	entry.TransactionID = "sale"
	result, err := ComputeFunnel(d, FunnelQuery{Start: funnelTime(0), End: funnelTime(1), Watermark: funnelTime(100)}, []AnalysisEvent{entry, funnelEvent("f", "A", "finish", 20)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Converted != 1 || result.Steps[1].Occurrences != 0 || len(result.Steps[1].AmountMinor) != 0 {
		t.Fatal(result)
	}
}

func TestFunnelParallelRejectsDuplicateEventCodes(t *testing.T) {
	d := basicFunnel()
	d.Steps[1].Events = nil
	d.Steps[1].AllEvents = []string{"paid", "paid"}
	if d.Validate() == nil {
		t.Fatal("duplicate parallel requirements must be rejected")
	}
}

func TestFunnelStepWindowExpiresBeforeTotalWindow(t *testing.T) {
	d := basicFunnel()
	d.Steps[1].WindowSeconds = 30
	r, err := ComputeFunnel(d, FunnelQuery{Start: funnelTime(0), End: funnelTime(1), Watermark: funnelTime(60)}, []AnalysisEvent{funnelEvent("s", "A", "start", 0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Steps[1].Expired != 1 || r.Steps[1].Pending != 0 {
		t.Fatalf("step deadline expired: %+v", r.Steps[1])
	}
}
func TestSubjectStateDoesNotInventHistoricalAttributeValues(t *testing.T) {
	defs := []SubjectStateDefinition{{Key: "active", SubjectType: "contract", Rule: StateRule{Kind: "attribute", Field: "active", Operator: "eq", Operand: true}}}
	ctx := StateEvaluationContext{At: funnelTime(100), Complete: true, Attributes: map[string]interface{}{"active": true}, Events: []AnalysisEvent{funnelEvent("s", "A", "start", 0)}}
	history, _, err := BuildSubjectStateTimeline(defs, ctx, nil, "contract", "A", "c")
	if err != nil {
		t.Fatal(err)
	}
	if SubjectStateAt(history, "contract", "A", "active", funnelTime(1)) != StateUnknown {
		t.Fatal("current attributes cannot be applied retroactively")
	}
}
func TestMarketingRuleAdapterGuardsObjectReplayAndLoop(t *testing.T) {
	rule := MarketingEventRule{Events: []string{"signed"}, SubjectType: "contract", OncePerObject: true, MaxCausationDepth: 4}
	e := funnelEvent("e", "B", "signed", 10)
	scope := MarketingRuleContext{Subject: EventSubject{Type: "contract", ID: "A"}, Now: funnelTime(20), ProjectionReady: true}
	if EvaluateMarketingEvent(rule, e, scope).Matched {
		t.Fatal("another contract cannot awaken waiting object")
	}
	scope.Subject.ID = "B"
	e.Backfill = true
	if EvaluateMarketingEvent(rule, e, scope).AllowSideEffects {
		t.Fatal("backfill cannot send")
	}
	e.Backfill = false
	scope.CausationDepth = 4
	if EvaluateMarketingEvent(rule, e, scope).AllowSideEffects {
		t.Fatal("causal loop must stop")
	}
	scope.CausationDepth = 0
	e.ExpiresAt = ptrTime(funnelTime(19))
	if EvaluateMarketingEvent(rule, e, scope).AllowSideEffects {
		t.Fatal("expired event cannot send")
	}
	e.ExpiresAt = nil
	result := EvaluateMarketingEvent(rule, e, scope)
	if !result.Matched || !result.AllowSideEffects || result.IdempotencyKey == "" {
		t.Fatal(result)
	}
	_ = time.Second
}

func TestMarketingRuleAdapterUsesProvidedOccurrenceContext(t *testing.T) {
	rule := MarketingEventRule{Events: []string{"login"}, Condition: &StateRule{Kind: "event", Event: "login", Minimum: 2, WindowSeconds: 60}}
	event := funnelEvent("two", "A", "login", 10)
	evaluation := StateEvaluationContext{At: funnelTime(1000), Complete: true, Events: []AnalysisEvent{funnelEvent("one", "A", "login", 0), event}}
	result := EvaluateMarketingEvent(rule, event, MarketingRuleContext{Now: funnelTime(20), ProjectionReady: true, Evaluation: &evaluation})
	if !result.AllowSideEffects {
		t.Fatal(result)
	}
	if !evaluation.At.Equal(funnelTime(1000)) {
		t.Fatal("caller context mutated")
	}
}

func TestFunnelORBranchesExposeOverlapWithoutDoubleCountingUnion(t *testing.T) {
	d := basicFunnel()
	d.Steps[1].Events = []string{"signed", "approved"}
	events := []AnalysisEvent{funnelEvent("a", "A", "start", 0), funnelEvent("b", "A", "signed", 10), funnelEvent("c", "A", "approved", 20)}
	r, err := ComputeFunnel(d, FunnelQuery{Start: funnelTime(0), End: funnelTime(1), Watermark: funnelTime(4000)}, events, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Steps[1].Objects != 1 || r.Steps[1].Branches["signed"] != 1 || r.Steps[1].Branches["approved"] != 1 {
		t.Fatal(r.Steps[1])
	}
}
