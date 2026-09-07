package domain

import (
	"testing"
	"time"
)

func TestTrackedEventNormalize(t *testing.T) {
	d := BusinessDefinition{Kind: "event", Key: "contract.signed", Name: "Signed", Status: "active", Revision: 2, Schema: map[string]any{"type": "object"}, Settings: map[string]any{"expiry_mode": "duration", "ttl_seconds": float64(60)}}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	raw := map[string]any{"report_id": "r1", "event_code": d.Key, "customer_attributes": map[string]any{"external_user_id": "x"}, "data": map[string]any{}, "triggered_at": now.Format(time.RFC3339)}
	e, err := NormalizeTrackedEvent(raw, &d, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if e.EventName != "Signed" || e.ExpiresAt == nil || !e.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("bad normalization: %+v", e)
	}
	delete(raw, "report_id")
	if _, err = NormalizeTrackedEvent(raw, &d, nil, now); err == nil {
		t.Fatal("missing report ID accepted")
	}
}
func TestTrackedEventNestedMapping(t *testing.T) {
	now := time.Now().UTC()
	d := BusinessDefinition{Key: "signed", Name: "Signed", Settings: map[string]any{"expiry_mode": "never"}}
	m := BusinessDefinition{Revision: 3, Settings: map[string]any{"event_code": "signed", "fields": map[string]any{"report_id": "$.meta.id", "data": "$.payload", "customer_attributes": "$.customer", "triggered_at": "$.at"}}}
	raw := map[string]any{"meta": map[string]any{"id": "one"}, "payload": map[string]any{}, "customer": map[string]any{"external_user_id": "one"}, "at": now.Format(time.RFC3339)}
	e, err := NormalizeTrackedEvent(raw, &d, &m, now)
	if err != nil || e.ReportID != "one" || e.MappingRevision != 3 {
		t.Fatalf("%+v %v", e, err)
	}
}
