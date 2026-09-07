package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ConfiguredJourneyStart checks the original occurrence at the execution boundary.
// Legacy journeys have no configured subject binding and keep their existing rules.
func (r *RealtimePostgresRepository) ConfiguredJourneyStart(ctx context.Context, w, journey string, now time.Time) (string, *time.Time, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return "", nil, err
	}
	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT e.properties FROM journey_event_subjects s JOIN event_ledger e ON e.id=s.origin_event_id WHERE s.contact_automation_id=$1 LIMIT 1`, journey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var metadata struct {
		ProcessingStatus string     `json:"processing_status"`
		Backfill         bool       `json:"backfill"`
		ExpiresAt        *time.Time `json:"expires_at"`
	}
	if err = json.Unmarshal(raw, &metadata); err != nil {
		return "", nil, err
	}
	if metadata.Backfill || metadata.ProcessingStatus != "" && metadata.ProcessingStatus != "accepted" {
		return "origin_event_ineligible", metadata.ExpiresAt, nil
	}
	if metadata.ExpiresAt != nil && !now.Before(*metadata.ExpiresAt) {
		return "origin_event_expired", metadata.ExpiresAt, nil
	}
	return "", metadata.ExpiresAt, nil
}
