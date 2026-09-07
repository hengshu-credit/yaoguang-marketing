package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

// Read all evidence under the caller's transaction. Object membership is independent
// of whether a state has ever been projected, and each child keeps its own facts.
func loadConfiguredRuleEvidence(ctx context.Context, tx *sql.Tx, event domain.AnalysisEvent, now time.Time, evaluation *domain.StateEvaluationContext) error {
	children := []domain.StateObjectContext{}
	rows, err := tx.QueryContext(ctx, `SELECT o.object_type,o.id::text,COALESCE(rel.relation_key,''),COALESCE(rel.valid_from,o.created_at),rel.valid_to FROM business_objects o LEFT JOIN business_object_relations rel ON rel.to_id=o.id AND rel.from_id::text=$3 WHERE o.customer_id=$1::uuid AND ($2='customer' OR rel.from_id IS NOT NULL) AND COALESCE(rel.valid_from,o.created_at)<=$4 AND (rel.valid_to IS NULL OR rel.valid_to>$4) ORDER BY o.id,rel.id LIMIT 10001`, event.CustomerID, event.SubjectType, event.SubjectID, event.OccurredAt)
	if err != nil {
		return err
	}
	for rows.Next() {
		var child domain.StateObjectContext
		if err = rows.Scan(&child.SubjectType, &child.SubjectID, &child.RelationKey, &child.ValidFrom, &child.ValidTo); err != nil {
			rows.Close()
			return err
		}
		child.States = map[string]domain.StateValue{}
		children = append(children, child)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(children) > 10000 {
		return fmt.Errorf("event condition child evidence exceeds 10000 objects")
	}
	byID := map[string][]int{}
	for i, c := range children {
		key := c.SubjectType + ":" + c.SubjectID
		byID[key] = append(byID[key], i)
	}
	rows, err = tx.QueryContext(ctx, `SELECT h.subject_type,h.subject_id,h.state_key,h.value FROM subject_state_history h JOIN subject_state_projections p ON p.subject_type=h.subject_type AND p.subject_id=h.subject_id AND p.revision=h.projection_revision WHERE h.customer_id=$1::uuid AND h.valid_from<=$2 AND (h.valid_to IS NULL OR h.valid_to>$2)`, event.CustomerID, event.OccurredAt)
	if err != nil {
		return err
	}
	for rows.Next() {
		var typ, id, key string
		var value domain.StateValue
		if err = rows.Scan(&typ, &id, &key, &value); err != nil {
			rows.Close()
			return err
		}
		if typ == event.SubjectType && id == event.SubjectID {
			evaluation.States[key] = value
		}
		for _, i := range byID[typ+":"+id] {
			children[i].States[key] = value
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT subject_type,subject_id,valid_from,recorded_at,attributes,changed_keys FROM subject_attribute_history WHERE customer_id=$1::uuid AND valid_from<=$2 AND recorded_at<=$3 ORDER BY valid_from,recorded_at,id LIMIT 100001`, event.CustomerID, event.OccurredAt, now)
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		count++
		var typ, id string
		var snapshot domain.StateAttributeSnapshot
		var raw, changed []byte
		if err = rows.Scan(&typ, &id, &snapshot.ValidFrom, &snapshot.RecordedAt, &raw, &changed); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &snapshot.Attributes); err != nil {
			rows.Close()
			return err
		}
		if len(changed) > 0 {
			if err = json.Unmarshal(changed, &snapshot.ChangedKeys); err != nil {
				rows.Close()
				return err
			}
		}
		if typ == event.SubjectType && id == event.SubjectID {
			evaluation.AttributeHistory = append(evaluation.AttributeHistory, snapshot)
		}
		for _, i := range byID[typ+":"+id] {
			children[i].AttributeHistory = append(children[i].AttributeHistory, snapshot)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count > 100000 {
		return fmt.Errorf("event condition attribute history exceeds 100000 snapshots")
	}
	attributes := domain.SubjectAttributesAt(evaluation.AttributeHistory, event.OccurredAt)
	if attributes == nil {
		attributes = map[string]any{}
	}
	for key, value := range evaluation.Attributes {
		attributes[key] = value
	}
	evaluation.Attributes = attributes
	for i := range children {
		children[i].Attributes = domain.SubjectAttributesAt(children[i].AttributeHistory, event.OccurredAt)
	}
	rows, err = tx.QueryContext(ctx, `SELECT id::text,subject_type,subject_id,event_type,occurred_at,received_at,properties FROM event_ledger WHERE customer_id=$1::uuid AND occurred_at<=$2 AND received_at<=$3 ORDER BY occurred_at,id LIMIT 100001`, event.CustomerID, event.OccurredAt, now)
	if err != nil {
		return err
	}
	defer rows.Close()
	evaluation.Events = nil
	count = 0
	for rows.Next() {
		count++
		e := domain.AnalysisEvent{CustomerID: event.CustomerID}
		var raw []byte
		if err = rows.Scan(&e.ID, &e.SubjectType, &e.SubjectID, &e.Code, &e.OccurredAt, &e.ReceivedAt, &raw); err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &e.Properties); err != nil {
			return err
		}
		e.Backfill = e.Properties["backfill"] == true || e.Properties["processing_status"] == "backfill"
		if value, ok := e.Properties["expires_at"].(string); ok && value != "" {
			at, parseErr := time.Parse(time.RFC3339Nano, value)
			if parseErr != nil {
				return parseErr
			}
			e.ExpiresAt = &at
		}
		if data, ok := e.Properties["data"].(map[string]any); ok {
			e.Properties = data
		}
		if event.SubjectType == "customer" || e.SubjectType == event.SubjectType && e.SubjectID == event.SubjectID {
			evaluation.Events = append(evaluation.Events, e)
		}
		for _, i := range byID[e.SubjectType+":"+e.SubjectID] {
			children[i].Events = append(children[i].Events, e)
		}
	}
	if count > 100000 {
		return fmt.Errorf("event condition history exceeds 100000 occurrences")
	}
	evaluation.Children = children
	return rows.Err()
}
