package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (r *ExecutionSnapshotPostgresRepository) DeliveryMediaReferences(ctx context.Context, ws, executionID, deliveryID string) (json.RawMessage, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT payload FROM execution_evidence WHERE execution_id=$1 AND evidence_key=$2`, executionID, "media:"+deliveryID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return raw, err
}
func (r *ExecutionSnapshotPostgresRepository) ExecutionReferencesAsset(ctx context.Context, ws, executionID, hash string) (bool, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return false, err
	}
	var found bool
	err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM execution_evidence WHERE execution_id=$1 AND kind='delivery_media' AND payload->'assets' @> jsonb_build_array(jsonb_build_object('hash',$2::text)))`, executionID, hash).Scan(&found)
	return found, err
}
func (r *ExecutionSnapshotPostgresRepository) ResolveMediaCustomer(ctx context.Context, ws, customerID, email string) (string, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return "", err
	}
	var id string
	err = db.QueryRowContext(ctx, `SELECT id::text FROM customers WHERE ($1<>'' AND id::text=$1) OR ($1='' AND id=(SELECT customer_id FROM contacts WHERE lower(email)=lower($2) LIMIT 1)) LIMIT 1`, customerID, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
