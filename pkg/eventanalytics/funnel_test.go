package eventanalytics

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"testing"
	"time"
)

func TestFunnelProjectionUsesNormalizedEnvelopeAndCanonicalCustomer(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()
	envelope := domain.EventEnvelope{EventID: id, Type: "signed", Subject: domain.EventSubject{Type: "contract", ID: "A", CustomerID: "canonical"}, OccurredAt: now, ReceivedAt: now, Data: json.RawMessage(`{"data":{"amount_minor":100,"currency":"CNY","transaction_id":"p"},"trusted_touchpoint_id":"t"}`)}
	event, err := AnalysisEventFromEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != id.String() || event.CustomerID != "canonical" || event.AmountMinor != 100 || event.TrustedTouchpointID != "t" {
		t.Fatal(event)
	}
}
