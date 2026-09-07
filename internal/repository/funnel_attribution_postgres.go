package repository

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *FunnelPostgresRepository) AttributionLinks(ctx context.Context, w string, start, end, watermark time.Time) ([]domain.AnalysisTouchpoint, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	// A queue row is not a delivered touchpoint. Confirmed provider submission
	// or an actual verified message click supplies the evidence used here.
	rows, err := db.QueryContext(ctx, `WITH delivered AS (
 SELECT DISTINCT ON (payload->>'message_id') payload->>'message_id' AS id,execution_id,customer_id,occurred_at
 FROM execution_evidence WHERE kind IN ('delivery_content','delivery_status') AND status='confirmed'
 AND customer_id IS NOT NULL AND COALESCE(payload->>'message_id','')<>'' AND occurred_at <= $3 ORDER BY payload->>'message_id',occurred_at
 ), issued AS (
 SELECT DISTINCT ON (payload->>'message_id') payload->>'message_id' AS id,execution_id,customer_id,occurred_at
 FROM execution_evidence WHERE kind IN ('delivery_content','delivery_status') AND customer_id IS NOT NULL
 AND COALESCE(payload->>'message_id','')<>'' AND occurred_at <= $3 ORDER BY payload->>'message_id',occurred_at
 )
 SELECT id,execution_id,customer_id::text,'delivery' AS kind,occurred_at FROM delivered WHERE occurred_at >= $1 AND occurred_at <= $2
 UNION ALL
 SELECT i.id,i.execution_id,i.customer_id::text,'click' AS kind,l.occurred_at FROM event_ledger l JOIN issued i
 ON i.id=COALESCE(l.properties->>'message_id',l.properties->'data'->>'message_id') AND i.customer_id=l.customer_id
 WHERE l.event_type='email.clicked' AND l.occurred_at >= $1 AND l.occurred_at <= $2 AND l.received_at <= $3
 AND COALESCE(l.properties->>'is_bot','false')<>'true' AND COALESCE(l.properties->>'is_scanner','false')<>'true'`, start, end, watermark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AnalysisTouchpoint{}
	for rows.Next() {
		var link domain.AnalysisTouchpoint
		if err := rows.Scan(&link.ID, &link.RunID, &link.CustomerID, &link.Kind, &link.At); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}
