package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func evaluateConfiguredTrigger(ctx context.Context, tx *sql.Tx, binding domain.TriggerBinding, compiled domain.CompiledTriggerCondition, envelope domain.EventEnvelope, now time.Time) (bool, error) {
	rule := compiled.Trigger.EventRule
	customer, err := resolveRuleEventCustomerTx(ctx, tx, envelope)
	if err != nil {
		return false, err
	}
	event := domain.AnalysisEvent{ID: envelope.EventID.String(), CustomerID: customer, SubjectType: envelope.Subject.Type, SubjectID: envelope.Subject.ID, Code: envelope.Type, OccurredAt: envelope.OccurredAt, ReceivedAt: envelope.ReceivedAt}
	if binding.SubjectType == "customer" {
		event.SubjectType = "customer"
		event.SubjectID = customer
	}
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return false, err
	}
	event.Properties = data
	if payload, ok := data["data"].(map[string]any); ok {
		event.Properties = payload
	}
	event.Backfill = data["processing_status"] == "backfill" || data["backfill"] == true
	if expiry, ok := data["expires_at"].(string); ok && expiry != "" {
		at, err := time.Parse(time.RFC3339Nano, expiry)
		if err != nil {
			return false, err
		}
		event.ExpiresAt = &at
	}
	if sourceAutomation, _ := data["automation_id"].(string); sourceAutomation == binding.AutomationID {
		return false, nil
	}
	if sourceAutomation, _ := event.Properties["automation_id"].(string); sourceAutomation == binding.AutomationID {
		return false, nil
	}
	if envelope.Subject.Type == "message_history" {
		var self bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM message_history WHERE id=$1 AND automation_id=$2)`, envelope.Subject.ID, binding.AutomationID).Scan(&self); err != nil {
			return false, err
		}
		if self {
			return false, nil
		}
	}
	depth := 0
	if n, ok := data["causation_depth"].(float64); ok {
		depth = int(n)
	}
	if envelope.CausationID != nil {
		if err := tx.QueryRowContext(ctx, `WITH RECURSIVE chain AS(SELECT id,context,1 AS depth,ARRAY[id] AS path FROM event_ledger WHERE id=$1 UNION ALL SELECT p.id,p.context,c.depth+1,c.path||p.id FROM chain c JOIN event_ledger p ON p.id::text=c.context->>'causation_id' WHERE c.depth<33 AND NOT p.id=ANY(c.path)) SELECT COALESCE(MAX(depth),0) FROM chain`, *envelope.CausationID).Scan(&depth); err != nil {
			return false, err
		}
	}
	evaluation := domain.StateEvaluationContext{At: envelope.OccurredAt, Attributes: map[string]any{}, States: map[string]domain.StateValue{}, Complete: true, Events: []domain.AnalysisEvent{event}}
	for key, value := range event.Properties {
		evaluation.Attributes[key] = value
	}
	evaluation.Attributes["data"] = event.Properties
	evaluation.Attributes["customer_attributes"] = data["customer_attributes"]
	if rule.Condition != nil {
		if err := loadConfiguredRuleEvidence(ctx, tx, event, now, &evaluation); err != nil {
			return false, err
		}
	}
	decision := domain.EvaluateMarketingEvent(*rule, event, domain.MarketingRuleContext{Now: now, ProjectionReady: true, Source: envelope.Source, AutomationID: binding.AutomationID, CausationDepth: depth, Evaluation: &evaluation})
	if !decision.Matched || !decision.AllowSideEffects {
		return false, nil
	}
	if rule.OncePerObject {
		key := binding.AutomationID + ":" + event.SubjectType + ":" + event.SubjectID
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
			return false, err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM marketing_trigger_subject_entries WHERE automation_id=$1 AND subject_type=$2 AND subject_id=$3)`, binding.AutomationID, event.SubjectType, event.SubjectID).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
	}
	return true, nil
}

func recordConfiguredEnrollment(ctx context.Context, tx *sql.Tx, binding domain.TriggerBinding, compiled domain.CompiledTriggerCondition, request domain.RuleProcessRequest, customerID, instanceID string) error {
	if compiled.Trigger == nil || compiled.Trigger.EventRule == nil {
		return nil
	}
	typ, id := request.Envelope.Subject.Type, request.Envelope.Subject.ID
	if binding.SubjectType == "customer" {
		typ, id = "customer", customerID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journey_event_subjects(contact_automation_id,subject_type,subject_id,customer_id,origin_event_id) VALUES($1,$2,$3,$4::uuid,$5) ON CONFLICT(contact_automation_id) DO NOTHING`, instanceID, typ, id, customerID, request.Envelope.EventID); err != nil {
		return err
	}
	if compiled.Trigger.EventRule.OncePerObject {
		_, err := tx.ExecContext(ctx, `INSERT INTO marketing_trigger_subject_entries(automation_id,subject_type,subject_id,customer_id,event_id,contact_automation_id) VALUES($1,$2,$3,$4::uuid,$5,$6) ON CONFLICT(automation_id,subject_type,subject_id) DO NOTHING`, binding.AutomationID, typ, id, customerID, request.Envelope.EventID, instanceID)
		return err
	}
	return nil
}
