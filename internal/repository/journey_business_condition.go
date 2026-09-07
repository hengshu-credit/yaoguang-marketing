package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
	"time"
)

func (r *RealtimePostgresRepository) EvaluateJourneyBusinessCondition(ctx context.Context, w, journey string, c domain.BusinessConditionNodeConfig, start, now time.Time) (bool, map[string]any, error) {
	if err := c.Validate(); err != nil {
		return false, nil, err
	}
	db, err := r.getDB(ctx, w)
	if err != nil {
		return false, nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback()
	var customer, subject, id string
	err = tx.QueryRowContext(ctx, `SELECT ca.customer_id::text,COALESCE(s.subject_type,'customer'),COALESCE(s.subject_id,ca.customer_id::text) FROM contact_automations ca LEFT JOIN journey_event_subjects s ON s.contact_automation_id=ca.id WHERE ca.id=$1`, journey).Scan(&customer, &subject, &id)
	if err != nil {
		return false, nil, err
	}
	if c.SubjectType == "customer" {
		subject = "customer"
		id = customer
	} else if c.SubjectType != subject {
		return false, nil, fmt.Errorf("condition subject must match enrolled business object")
	}
	evidence := map[string]any{"subject_type": subject, "subject_id": id, "customer_id": customer}
	evaluate := func(at time.Time, event domain.AnalysisEvent) (bool, error) {
		if c.Condition == nil {
			return true, nil
		}
		event.CustomerID = customer
		event.SubjectType = subject
		event.SubjectID = id
		event.OccurredAt = at
		evaluation := domain.StateEvaluationContext{At: at, Attributes: map[string]any{}, States: map[string]domain.StateValue{}, Complete: true}
		for key, val := range event.Properties {
			evaluation.Attributes[key] = val
		}
		evaluation.Attributes["data"] = event.Properties
		if err := loadConfiguredRuleEvidence(ctx, tx, event, now, &evaluation); err != nil {
			return false, err
		}
		result := domain.EvaluateStateRule(*c.Condition, evaluation)
		evidence["condition_value"] = result.Value
		return result.Value == domain.StateTrue, nil
	}
	if len(c.Events) == 0 {
		ok, err := evaluate(now, domain.AnalysisEvent{})
		return ok, evidence, err
	}
	until := now
	if c.Mode == "wait" {
		deadline := start.Add(time.Duration(c.TimeoutSeconds) * time.Second)
		if deadline.Before(until) {
			until = deadline
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id::text,event_type,occurred_at,received_at,properties FROM event_ledger WHERE customer_id=$1::uuid AND ($2='customer' OR (subject_type=$2 AND subject_id=$3)) AND event_type=ANY($4) AND occurred_at >= $5 AND occurred_at <= $6 AND received_at <= $7 ORDER BY occurred_at,id LIMIT 1001`, customer, subject, id, pq.Array(c.Events), start, until, now)
	if err != nil {
		return false, nil, err
	}
	var candidates []domain.AnalysisEvent
	for rows.Next() {
		var event domain.AnalysisEvent
		var raw []byte
		if err = rows.Scan(&event.ID, &event.Code, &event.OccurredAt, &event.ReceivedAt, &raw); err != nil {
			rows.Close()
			return false, nil, err
		}
		if err = json.Unmarshal(raw, &event.Properties); err != nil {
			rows.Close()
			return false, nil, err
		}
		candidates = append(candidates, event)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, nil, err
	}
	if len(candidates) > 1000 {
		return false, nil, fmt.Errorf("wait event history exceeds 1000 candidates")
	}
	for _, event := range candidates {
		if status, _ := event.Properties["processing_status"].(string); status != "" && status != "accepted" {
			continue
		}
		if event.Properties["backfill"] == true {
			continue
		}
		if expires, _ := event.Properties["expires_at"].(string); expires != "" {
			at, err := time.Parse(time.RFC3339Nano, expires)
			if err != nil {
				return false, nil, err
			}
			if !now.Before(at) {
				continue
			}
		}
		if payload, ok := event.Properties["data"].(map[string]any); ok {
			event.Properties = payload
		}
		matched, err := evaluate(event.OccurredAt, event)
		if err != nil {
			return false, nil, err
		}
		if matched {
			evidence["event_id"] = event.ID
			evidence["event_code"] = event.Code
			return true, evidence, nil
		}
	}
	return false, evidence, nil
}
