package repository

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *SubjectStatePostgresRepository) AttributeHistory(ctx context.Context, w, typ, id string, watermark time.Time) ([]domain.StateAttributeSnapshot, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT valid_from,recorded_at,attributes,changed_keys FROM subject_attribute_history WHERE subject_type=$1 AND subject_id=$2 AND recorded_at <= $3 ORDER BY valid_from,id`, typ, id, watermark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.StateAttributeSnapshot{}
	for rows.Next() {
		var h domain.StateAttributeSnapshot
		var raw, keys []byte
		if err := rows.Scan(&h.ValidFrom, &h.RecordedAt, &raw, &keys); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &h.Attributes); err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			if err = json.Unmarshal(keys, &h.ChangedKeys); err != nil {
				return nil, err
			}
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
