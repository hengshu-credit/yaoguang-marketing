package repository

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

// A saved query's receipt watermark also bounds the state projection revision.
// Late rebuilds remain available to new queries without rewriting old reports.
func (r *FunnelPostgresRepository) StateHistoryAt(ctx context.Context, w string, start, end, watermark time.Time) ([]domain.SubjectStateInterval, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `WITH eligible AS (SELECT subject_type,subject_id,projection_revision FROM subject_state_history GROUP BY subject_type,subject_id,projection_revision HAVING MAX(computed_at) <= $3), selected AS (SELECT subject_type,subject_id,MAX(projection_revision) AS revision FROM eligible GROUP BY subject_type,subject_id) SELECT h.subject_type,h.subject_id,COALESCE(h.customer_id::text,''),h.state_key,h.value,h.valid_from,h.valid_to,h.rule_revision,h.projection_revision,h.source,h.reason,h.event_ids,h.computed_at FROM subject_state_history h JOIN selected p ON p.subject_type=h.subject_type AND p.subject_id=h.subject_id AND p.revision=h.projection_revision WHERE h.valid_from <= $2 AND (h.valid_to IS NULL OR h.valid_to > $1)`, start, end, watermark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubjectStates(rows)
}
func (r *FunnelClickHouseRepository) StateHistoryAt(ctx context.Context, w string, start, end, watermark time.Time) ([]domain.SubjectStateInterval, error) {
	return r.source.StateHistoryAt(ctx, w, start, end, watermark)
}
