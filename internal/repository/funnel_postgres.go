package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type FunnelPostgresRepository struct{ base *RealtimePostgresRepository }

func NewFunnelRepository(w domain.WorkspaceRepository) *FunnelPostgresRepository {
	return &FunnelPostgresRepository{base: NewRealtimeRepository(w)}
}
func NewFunnelRepositoryWithDB(db *sql.DB) *FunnelPostgresRepository {
	return &FunnelPostgresRepository{base: NewRealtimeRepositoryWithDB(db)}
}
func (r *FunnelPostgresRepository) Events(ctx context.Context, w, subjectType string, start, end, watermark time.Time) ([]domain.AnalysisEvent, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id::text,event_type,subject_type,subject_id,COALESCE(customer_id::text,''),occurred_at,received_at,sequence,properties FROM event_ledger WHERE occurred_at >= $1 AND occurred_at <= $2 AND received_at <= $3 AND ($4 = 'customer' OR subject_type = $4) ORDER BY occurred_at,id LIMIT $5`, start, end, watermark, subjectType, 100001)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnalysisEvents(rows)
}
func (r *FunnelPostgresRepository) EventsForSubject(ctx context.Context, w, typ, id string, start, end, watermark time.Time) ([]domain.AnalysisEvent, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id::text,event_type,subject_type,subject_id,COALESCE(customer_id::text,''),occurred_at,received_at,sequence,properties FROM event_ledger WHERE occurred_at >= $1 AND occurred_at <= $2 AND received_at <= $3 AND (($4='customer' AND customer_id=NULLIF($5,'')::uuid) OR ($4<>'customer' AND subject_type=$4 AND subject_id=$5)) ORDER BY occurred_at,id LIMIT 100001`, start, end, watermark, typ, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnalysisEvents(rows)
}
func scanAnalysisEvents(rows *sql.Rows) ([]domain.AnalysisEvent, error) {
	out := []domain.AnalysisEvent{}
	for rows.Next() {
		var e domain.AnalysisEvent
		var raw []byte
		if err := rows.Scan(&e.ID, &e.Code, &e.SubjectType, &e.SubjectID, &e.CustomerID, &e.OccurredAt, &e.ReceivedAt, &e.Sequence, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &e.Properties); err != nil {
			return nil, fmt.Errorf("decode ledger properties: %w", err)
		}
		envelope := e.Properties
		e.TrustedTouchpointID, _ = envelope["trusted_touchpoint_id"].(string)
		if data, ok := envelope["data"].(map[string]interface{}); ok {
			e.Properties = data
		}
		if n, ok := e.Properties["amount_minor"].(float64); ok {
			e.AmountMinor = int64(n)
		}
		e.Currency, _ = e.Properties["currency"].(string)
		e.TransactionID, _ = e.Properties["transaction_id"].(string)
		if expiry, ok := envelope["expires_at"].(string); ok {
			at, err := time.Parse(time.RFC3339Nano, expiry)
			if err == nil {
				e.ExpiresAt = &at
			}
		}
		e.Backfill, _ = envelope["backfill"].(bool)
		out = append(out, e)
	}
	if len(out) > 100000 {
		return nil, fmt.Errorf("query exceeds 100000 events; narrow the date range")
	}
	return out, rows.Err()
}
func (r *FunnelPostgresRepository) StateHistory(ctx context.Context, w string, start, end time.Time) ([]domain.SubjectStateInterval, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT h.subject_type,h.subject_id,COALESCE(h.customer_id::text,''),h.state_key,h.value,h.valid_from,h.valid_to,h.rule_revision,h.projection_revision,h.source,h.reason,h.event_ids,h.computed_at FROM subject_state_history h JOIN subject_state_projections p ON p.subject_type=h.subject_type AND p.subject_id=h.subject_id AND p.revision=h.projection_revision WHERE h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $1)`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubjectStates(rows)
}
func scanSubjectStates(rows *sql.Rows) ([]domain.SubjectStateInterval, error) {
	out := []domain.SubjectStateInterval{}
	for rows.Next() {
		var h domain.SubjectStateInterval
		var ids []byte
		if err := rows.Scan(&h.SubjectType, &h.SubjectID, &h.CustomerID, &h.StateKey, &h.Value, &h.ValidFrom, &h.ValidTo, &h.RuleRevision, &h.ProjectionRevision, &h.Source, &h.Reason, &ids, &h.ComputedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(ids, &h.EventIDs); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
