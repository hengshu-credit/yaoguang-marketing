package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *ContactEndpointPostgresRepository) SaveCustomerEndpoint(ctx context.Context, request *domain.CustomerEndpointRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	endpoint, err := request.Endpoint.Validate()
	if err != nil {
		return err
	}
	db, err := r.getDB(ctx, request.WorkspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var actual string
	if err = tx.QueryRowContext(ctx, `SELECT id::text FROM customers WHERE id=$1::uuid AND merged_into_id IS NULL FOR UPDATE`, request.CustomerID).Scan(&actual); err != nil {
		return err
	}
	if request.Endpoint.Operation == domain.EndpointOperationDisable {
		_, err = tx.ExecContext(ctx, `UPDATE contact_endpoints SET enabled=false,updated_at=CURRENT_TIMESTAMP,version=version+1 WHERE endpoint_id=$1 AND customer_id=$2::uuid`, request.Endpoint.EndpointID, actual)
	} else {
		ciphertext, encryptErr := r.encryptAddress(endpoint.Address)
		if encryptErr != nil {
			return encryptErr
		}
		attributes, _ := json.Marshal(endpoint.Attributes)
		if endpoint.Attributes == nil {
			attributes = []byte(`{}`)
		}
		result, execErr := tx.ExecContext(ctx, `INSERT INTO contact_endpoints(endpoint_id,customer_id,email,channel,provider,platform,address_ciphertext,address_fingerprint,locale,timezone,app_id,device_id,attributes,enabled) VALUES($1,$2::uuid,NULL,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12,true) ON CONFLICT(endpoint_id) DO UPDATE SET channel=EXCLUDED.channel,provider=EXCLUDED.provider,platform=EXCLUDED.platform,address_ciphertext=EXCLUDED.address_ciphertext,address_fingerprint=EXCLUDED.address_fingerprint,locale=EXCLUDED.locale,timezone=EXCLUDED.timezone,attributes=EXCLUDED.attributes,enabled=true,updated_at=CURRENT_TIMESTAMP,version=contact_endpoints.version+1 WHERE contact_endpoints.customer_id=EXCLUDED.customer_id`, endpoint.EndpointID, actual, endpoint.Channel, endpoint.Provider, endpoint.Platform, ciphertext, endpointFingerprint(endpoint.Address), endpoint.Locale, endpoint.Timezone, endpoint.AppID, endpoint.DeviceID, attributes)
		if execErr != nil {
			return execErr
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return fmt.Errorf("endpoint belongs to another customer")
		}
	}
	if err != nil {
		return err
	}
	if request.MarketingConsent != nil {
		status := "denied"
		if *request.MarketingConsent {
			status = "granted"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO customer_consents(id,customer_id,purpose,channel,status,source,revoked_at) VALUES($1,$2::uuid,'marketing',$3,$4,'customer_endpoint_form',CASE WHEN $5 THEN NULL ELSE CURRENT_TIMESTAMP END) ON CONFLICT(customer_id,purpose,channel) DO UPDATE SET status=EXCLUDED.status,source=EXCLUDED.source,revoked_at=EXCLUDED.revoked_at,valid_from=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, uuid.NewString(), actual, request.Endpoint.Channel, status, *request.MarketingConsent)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO customer_endpoint_consent_audit(id,customer_id,channel,granted,actor_id,source) VALUES($1,$2::uuid,$3,$4,$5,'customer_endpoint_form')`, uuid.NewString(), actual, request.Endpoint.Channel, *request.MarketingConsent, request.ActorID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (r *ContactEndpointPostgresRepository) ListCustomerEndpoints(ctx context.Context, ws, id, channel string) ([]*domain.ContactEndpoint, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT to_jsonb(e)-'address_ciphertext'-'address_fingerprint' FROM contact_endpoints e WHERE customer_id=$1::uuid AND ($2='' OR channel=$2) ORDER BY updated_at DESC,endpoint_id`, id, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.ContactEndpoint{}
	for rows.Next() {
		var raw []byte
		var endpoint domain.ContactEndpoint
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &endpoint); err != nil {
			return nil, err
		}
		out = append(out, &endpoint)
	}
	return out, rows.Err()
}
