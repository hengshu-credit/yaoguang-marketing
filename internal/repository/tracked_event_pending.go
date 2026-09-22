package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

// ResolvePending reuses the first normalized payload and definition snapshots.
// Only the identity resolution outcome changes; it never creates another ledger fact.
func (r *TrackedEventPostgresRepository) ResolvePending(ctx context.Context, w, source, report, requestedCustomer, reason string, now time.Time) (*domain.TrackedEventResult, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var eventID, status string
	var normalized, definition, mapping, attributes, object, response []byte
	if err = tx.QueryRowContext(ctx, `SELECT event_id,processing_status,normalized_event,definition_snapshot,mapping_snapshot,attribute_definitions,object_definition_snapshot,response FROM event_report_receipts WHERE source=$1 AND report_id=$2 FOR UPDATE`, source, report).Scan(&eventID, &status, &normalized, &definition, &mapping, &attributes, &object, &response); err != nil {
		return nil, err
	}
	if status != "pending_identity" {
		var result domain.TrackedEventResult
		if err = json.Unmarshal(response, &result); err != nil {
			return nil, err
		}
		if result.CustomerID != "" || status != "expired" && status != "backfill" && status != "associated_history" {
			result.Duplicate = true
			return &result, tx.Commit()
		}
	}
	var event domain.TrackedEvent
	if err = json.Unmarshal(normalized, &event); err != nil {
		return nil, err
	}
	command := domain.TrackedEventCommand{WorkspaceID: w, Source: source, Event: &event}
	if err = json.Unmarshal(definition, &command.Definition); err != nil {
		return nil, err
	}
	if command.Definition == nil || command.Definition.Key == "" {
		return nil, domain.NewValidationError("original event definition snapshot is missing")
	}
	if len(mapping) > 0 {
		if err = json.Unmarshal(mapping, &command.Mapping); err != nil {
			return nil, err
		}
	}
	if len(object) > 0 {
		if err = json.Unmarshal(object, &command.ObjectDefinition); err != nil {
			return nil, err
		}
	}
	if err = json.Unmarshal(attributes, &command.Attributes); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(eventID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('notifuse.business_event_at',$1,true),set_config('notifuse.business_event_id',$2,true)`, event.TriggeredAt.Format(time.RFC3339Nano), eventID); err != nil {
		return nil, err
	}
	if status == "pending_identity" {
		event.ProcessingStatus = "accepted"
	}
	applyTrackedEventCutoff(&event, now)
	identityEvent := event
	identityEvent.CustomerAttributes = map[string]any{}
	for k, v := range event.CustomerAttributes {
		identityEvent.CustomerAttributes[k] = v
	}
	if requestedCustomer != "" {
		identityEvent.CustomerAttributes["customer_id"] = requestedCustomer
	}
	command.Event = &identityEvent
	customer, err := r.resolveTrackedCustomer(ctx, tx, command)
	if err != nil {
		return nil, err
	}
	if customer == "" {
		return nil, domain.NewValidationError("customer identity remains unresolved")
	}
	command.Event = &event
	subject, err := resolveTrackedSubject(ctx, tx, command, customer, id)
	if err != nil {
		return nil, err
	}
	normalized, err = json.Marshal(event)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE event_ledger SET customer_id=$2::uuid,subject_type=$3,subject_id=$4,properties=$5 WHERE id=$1 AND received_at=$6`, id, customer, subject.Type, subject.ID, normalized, event.ReportedAt); err != nil {
		return nil, err
	}
	result := &domain.TrackedEventResult{Source: source, ReportID: event.ReportID, EventID: eventID, CustomerID: customer, ReportedAt: event.ReportedAt, ProcessingStatus: event.ProcessingStatus, Event: &event}
	response, _ = json.Marshal(result)
	if _, err = tx.ExecContext(ctx, `UPDATE event_report_receipts SET customer_id=$3::uuid,processing_status=$4,normalized_event=$5,response=$6,resolved_at=$7,resolution_reason=$8 WHERE source=$1 AND report_id=$2`, source, report, customer, event.ProcessingStatus, normalized, response, now, reason); err != nil {
		return nil, err
	}
	if err = markBusinessProjectionDirty(ctx, tx, "customer", customer); err != nil {
		return nil, err
	}
	if err = markBusinessProjectionDirty(ctx, tx, subject.Type, subject.ID); err != nil {
		return nil, err
	}
	if event.ProcessingStatus == "accepted" {
		releaseID := uuid.NewSHA1(id, []byte("identity-resolution:v1"))
		envelope := domain.EventEnvelope{ID: releaseID, EventID: id, Type: event.EventCode, SchemaVersion: 1, WorkspaceID: w, Subject: subject, Source: source, OccurredAt: event.TriggeredAt, ReceivedAt: event.ReportedAt, CorrelationID: id, Data: normalized}
		payload, err := json.Marshal(envelope)
		if err != nil {
			return nil, err
		}
		headers, _ := json.Marshal(map[string]any{"identity_resolution": true, "reason": reason})
		if _, err = tx.ExecContext(ctx, `INSERT INTO event_outbox(id,event_id,topic,routing_key,payload,headers) VALUES($1,$2,'notifuse.events',$3,$4,$5) ON CONFLICT(event_id,topic,routing_key) DO NOTHING`, releaseID, id, event.EventCode+".identity_resolved", payload, headers); err != nil {
			return nil, fmt.Errorf("release resolved event: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
