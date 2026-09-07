package repository

import (
	"context"
	"encoding/json"
	"github.com/lib/pq"
	"time"
)

// Association requires server-signed session evidence. Historical occurrences are
// linked for analysis only; no old event is replayed through marketing triggers.
func (r *TrackedEventPostgresRepository) AssociateBrowserSession(ctx context.Context, w, source, session, customer string, codes []string) (int, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT event_id,reported_at,response FROM event_report_receipts WHERE source=$1 AND customer_id IS NULL AND normalized_event->'customer_attributes'->>'browser_session_id'=$2 AND normalized_event->>'event_code'=ANY($3::text[]) FOR UPDATE`, source, session, pq.Array(codes))
	if err != nil {
		return 0, err
	}
	type item struct {
		id       string
		at       time.Time
		response []byte
	}
	items := []item{}
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.id, &value.at, &value.response); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, value := range items {
		var response map[string]any
		if err = json.Unmarshal(value.response, &response); err != nil {
			return 0, err
		}
		response["customer_id"] = customer
		response["processing_status"] = "associated_history"
		if event, ok := response["event"].(map[string]any); ok {
			event["processing_status"] = "associated_history"
		}
		encoded, _ := json.Marshal(response)
		if _, err = tx.ExecContext(ctx, `UPDATE event_report_receipts SET customer_id=$2,processing_status='associated_history',response=$3 WHERE event_id=$1`, value.id, customer, encoded); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE event_ledger SET customer_id=$2::uuid,subject_type='customer',subject_id=$2::text,properties=jsonb_set(properties,'{processing_status}','"associated_history"'::jsonb) WHERE id=$1 AND received_at=$3`, value.id, customer, value.at); err != nil {
			return 0, err
		}
	}
	if err = markBusinessProjectionDirty(ctx, tx, "customer", customer); err != nil {
		return 0, err
	}
	return len(items), tx.Commit()
}
