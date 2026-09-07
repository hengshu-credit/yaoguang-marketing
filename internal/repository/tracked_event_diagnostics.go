package repository

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

// FindReceiptByPayload preserves the first interpretation when a mapping path changes.
func (r *TrackedEventPostgresRepository) FindReceiptByPayload(ctx context.Context, w, source, hash string) (*domain.TrackedEventResult, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	var erased bool
	if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM business_erased_reports WHERE source_hash=md5($1) AND payload_hash=$2)`, source, hash).Scan(&erased); err != nil {
		return nil, err
	}
	if erased {
		return nil, domain.ErrTrackedEventErased
	}
	var raw []byte
	if err = db.QueryRowContext(ctx, `SELECT response FROM event_report_receipts WHERE source=$1 AND payload_hash=$2 ORDER BY reported_at LIMIT 1`, source, hash).Scan(&raw); err != nil {
		return nil, err
	}
	var result domain.TrackedEventResult
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	result.Duplicate = true
	return &result, nil
}
func (r *TrackedEventPostgresRepository) RecordDiagnostic(ctx context.Context, w, source, report, code, message string) error {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO event_ingest_diagnostics(id,source,report_id,event_code,error_code,message) VALUES(gen_random_uuid(),$1,$2,$3,'validation',$4)`, source, report, code, message)
	return err
}

func (r *TrackedEventPostgresRepository) ListDiagnostics(ctx context.Context, w, source string, limit int) ([]map[string]any, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `SELECT id,source,report_id,event_code,error_code,message,created_at FROM event_ingest_diagnostics WHERE ($1='' OR source=$1) ORDER BY created_at DESC LIMIT $2`, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, src, report, code, errorCode, message string
		var at time.Time
		if err = rows.Scan(&id, &src, &report, &code, &errorCode, &message, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "source": src, "report_id": report, "event_code": code, "error_code": errorCode, "message": message, "created_at": at})
	}
	return out, rows.Err()
}
