package eventanalytics

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func AnalysisEventFromEnvelope(envelope domain.EventEnvelope) (domain.AnalysisEvent, error) {
	event := domain.AnalysisEvent{ID: envelope.EventID.String(), CustomerID: envelope.Subject.CustomerID, SubjectType: envelope.Subject.Type, SubjectID: envelope.Subject.ID, Code: envelope.Type, OccurredAt: envelope.OccurredAt, ReceivedAt: envelope.ReceivedAt}
	var properties map[string]interface{}
	if err := json.Unmarshal(envelope.Data, &properties); err != nil {
		return event, err
	}
	event.Properties = properties
	if data, ok := properties["data"].(map[string]interface{}); ok {
		event.Properties = data
	}
	event.TrustedTouchpointID, _ = properties["trusted_touchpoint_id"].(string)
	event.Backfill, _ = properties["backfill"].(bool)
	if raw, ok := properties["expires_at"].(string); ok {
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil {
			event.ExpiresAt = &at
		}
	}
	if amount, ok := event.Properties["amount_minor"].(float64); ok {
		event.AmountMinor = int64(amount)
	}
	event.Currency, _ = event.Properties["currency"].(string)
	event.TransactionID, _ = event.Properties["transaction_id"].(string)
	return event, nil
}

// Events reads the logically deduplicated projection. Both readers use the
// occurrence-time range and receipt watermark; envelope_json preserves Customer
// UUID and timestamp precision absent from the original flattened CH columns.
func (s *ClickHouseStore) Events(ctx context.Context, w, subjectType string, start, end, watermark time.Time) ([]domain.AnalysisEvent, error) {
	rows, err := s.connection.Query(ctx, fmt.Sprintf(`SELECT envelope_json FROM %s.event_projection FINAL WHERE workspace_id=? AND occurred_at>=? AND occurred_at<=? AND received_at<=? AND (?='customer' OR subject_type=?) ORDER BY occurred_at,event_id LIMIT 100001`, s.database), w, start, end, watermark, subjectType, subjectType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []domain.AnalysisEvent{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var envelope domain.EventEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			return nil, err
		}
		event, err := AnalysisEventFromEnvelope(envelope)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(events) > 100000 {
		return nil, fmt.Errorf("query exceeds 100000 events; narrow the date range")
	}
	return events, rows.Err()
}
