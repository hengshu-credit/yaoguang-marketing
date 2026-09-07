package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

// ResolveCustomerChannel uses explicit Customer ownership and active consent;
// no email or guessed identity is needed to select an endpoint.
func (r *ContactEndpointPostgresRepository) ResolveCustomerChannel(ctx context.Context, ws, id, channel string) (*domain.Customer, []*domain.ContactEndpoint, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, nil, err
	}
	var profile []byte
	customer := &domain.Customer{ID: id}
	err = db.QueryRowContext(ctx, `SELECT c.customer_no,to_jsonb(p) FROM customers c LEFT JOIN customer_profiles p ON p.customer_id=c.id WHERE c.id=$1::uuid AND c.merged_into_id IS NULL`, id).Scan(&customer.CustomerNo, &profile)
	if err != nil {
		return nil, nil, err
	}
	if len(profile) > 0 {
		if err = json.Unmarshal(profile, &customer.Profile); err != nil {
			return nil, nil, err
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT endpoint_id,COALESCE(email,''),channel,provider,platform,address_ciphertext,locale,timezone,app_id,device_id,attributes FROM contact_endpoints WHERE customer_id=$1::uuid AND channel=$2 AND enabled AND EXISTS(SELECT 1 FROM customer_consents c WHERE c.customer_id=$1::uuid AND c.channel=$2 AND c.purpose='marketing' AND c.status IN ('granted','active') AND c.valid_from<=CURRENT_TIMESTAMP AND (c.revoked_at IS NULL OR c.revoked_at>CURRENT_TIMESTAMP)) ORDER BY last_seen_at DESC,endpoint_id`, id, channel)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []*domain.ContactEndpoint{}
	for rows.Next() {
		endpoint := &domain.ContactEndpoint{Enabled: true}
		var ciphertext string
		var locale, timezone, app, device sql.NullString
		var attributes []byte
		if err = rows.Scan(&endpoint.EndpointID, &endpoint.Email, &endpoint.Channel, &endpoint.Provider, &endpoint.Platform, &ciphertext, &locale, &timezone, &app, &device, &attributes); err != nil {
			return nil, nil, err
		}
		endpoint.Address, err = pkgcrypto.DecryptFromHexString(ciphertext, r.secretKey)
		if err != nil {
			return nil, nil, fmt.Errorf("decrypt customer endpoint: %w", err)
		}
		endpoint.Locale = locale.String
		endpoint.Timezone = timezone.String
		endpoint.AppID = app.String
		endpoint.DeviceID = device.String
		if err = json.Unmarshal(attributes, &endpoint.Attributes); err != nil {
			return nil, nil, err
		}
		out = append(out, endpoint)
	}
	return customer, out, rows.Err()
}
