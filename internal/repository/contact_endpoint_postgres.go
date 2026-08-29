package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

type ContactEndpointPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
	secretKey     string
}

func NewContactEndpointRepository(
	workspaceRepo domain.WorkspaceRepository,
	secretKey string,
) (*ContactEndpointPostgresRepository, error) {
	if workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	if secretKey == "" {
		return nil, errors.New("contact endpoint encryption secret is required")
	}
	return &ContactEndpointPostgresRepository{workspaceRepo: workspaceRepo, secretKey: secretKey}, nil
}

func NewContactEndpointRepositoryWithDB(
	db *sql.DB,
	secretKey string,
) (*ContactEndpointPostgresRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if secretKey == "" {
		return nil, errors.New("contact endpoint encryption secret is required")
	}
	return &ContactEndpointPostgresRepository{db: db, secretKey: secretKey}, nil
}

func (r *ContactEndpointPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection: %w", err)
	}
	return db, nil
}

func (r *ContactEndpointPostgresRepository) encryptAddress(address string) (string, error) {
	encrypted, err := pkgcrypto.EncryptString(address, r.secretKey)
	if err != nil {
		return "", fmt.Errorf("encrypt contact endpoint address: %w", err)
	}
	return encrypted, nil
}

func endpointFingerprint(address string) string {
	sum := sha256.Sum256([]byte(address))
	return hex.EncodeToString(sum[:])
}

func (r *ContactEndpointPostgresRepository) Upsert(
	ctx context.Context,
	workspaceID, email string,
	endpoint *domain.ContactEndpoint,
) error {
	if endpoint == nil {
		return errors.New("contact endpoint is required")
	}
	if email == "" {
		return errors.New("contact endpoint email is required")
	}
	validated, err := (domain.ContactEndpointMutation{
		Operation: domain.EndpointOperationUpsert, EndpointID: endpoint.EndpointID,
		Channel: endpoint.Channel, Provider: endpoint.Provider, Platform: endpoint.Platform,
		Address: endpoint.Address, Locale: endpoint.Locale, Timezone: endpoint.Timezone,
		AppID: endpoint.AppID, DeviceID: endpoint.DeviceID, Attributes: endpoint.Attributes,
	}).Validate()
	if err != nil {
		return fmt.Errorf("invalid contact endpoint: %w", err)
	}
	ciphertext, err := r.encryptAddress(validated.Address)
	if err != nil {
		return err
	}
	attributesJSON, err := json.Marshal(validated.Attributes)
	if err != nil {
		return fmt.Errorf("marshal contact endpoint attributes: %w", err)
	}
	if validated.Attributes == nil {
		attributesJSON = []byte("{}")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO contact_endpoints (
			endpoint_id, email, channel, provider, platform,
			address_ciphertext, address_fingerprint, locale, timezone,
			app_id, device_id, attributes, enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''),
			NULLIF($10, ''), NULLIF($11, ''), $12, $13)
		ON CONFLICT (endpoint_id) DO UPDATE SET
			email = EXCLUDED.email,
			channel = EXCLUDED.channel,
			provider = EXCLUDED.provider,
			platform = EXCLUDED.platform,
			address_ciphertext = CASE
				WHEN contact_endpoints.address_fingerprint IS DISTINCT FROM EXCLUDED.address_fingerprint
				THEN EXCLUDED.address_ciphertext ELSE contact_endpoints.address_ciphertext END,
			address_fingerprint = EXCLUDED.address_fingerprint,
			locale = COALESCE(EXCLUDED.locale, contact_endpoints.locale),
			timezone = COALESCE(EXCLUDED.timezone, contact_endpoints.timezone),
			app_id = COALESCE(EXCLUDED.app_id, contact_endpoints.app_id),
			device_id = COALESCE(EXCLUDED.device_id, contact_endpoints.device_id),
			attributes = contact_endpoints.attributes || EXCLUDED.attributes,
			enabled = TRUE,
			version = contact_endpoints.version + CASE WHEN
				ROW(contact_endpoints.email, contact_endpoints.channel, contact_endpoints.provider,
					contact_endpoints.platform, contact_endpoints.address_fingerprint,
					contact_endpoints.locale, contact_endpoints.timezone, contact_endpoints.app_id,
					contact_endpoints.device_id, contact_endpoints.attributes, contact_endpoints.enabled)
				IS DISTINCT FROM
				ROW(EXCLUDED.email, EXCLUDED.channel, EXCLUDED.provider, EXCLUDED.platform,
					EXCLUDED.address_fingerprint, COALESCE(EXCLUDED.locale, contact_endpoints.locale),
					COALESCE(EXCLUDED.timezone, contact_endpoints.timezone),
					COALESCE(EXCLUDED.app_id, contact_endpoints.app_id),
					COALESCE(EXCLUDED.device_id, contact_endpoints.device_id),
					contact_endpoints.attributes || EXCLUDED.attributes, TRUE)
				THEN 1 ELSE 0 END,
			updated_at = CASE WHEN
				ROW(contact_endpoints.email, contact_endpoints.channel, contact_endpoints.provider,
					contact_endpoints.platform, contact_endpoints.address_fingerprint,
					contact_endpoints.locale, contact_endpoints.timezone, contact_endpoints.app_id,
					contact_endpoints.device_id, contact_endpoints.attributes, contact_endpoints.enabled)
				IS DISTINCT FROM
				ROW(EXCLUDED.email, EXCLUDED.channel, EXCLUDED.provider, EXCLUDED.platform,
					EXCLUDED.address_fingerprint, COALESCE(EXCLUDED.locale, contact_endpoints.locale),
					COALESCE(EXCLUDED.timezone, contact_endpoints.timezone),
					COALESCE(EXCLUDED.app_id, contact_endpoints.app_id),
					COALESCE(EXCLUDED.device_id, contact_endpoints.device_id),
					contact_endpoints.attributes || EXCLUDED.attributes, TRUE)
				THEN CURRENT_TIMESTAMP ELSE contact_endpoints.updated_at END,
			last_seen_at = CURRENT_TIMESTAMP
	`, validated.EndpointID, email, validated.Channel, validated.Provider, validated.Platform,
		ciphertext, endpointFingerprint(validated.Address), validated.Locale, validated.Timezone,
		validated.AppID, validated.DeviceID, string(attributesJSON), true); err != nil {
		return fmt.Errorf("upsert contact endpoint %s: %w", validated.EndpointID, err)
	}
	return nil
}

func (r *ContactEndpointPostgresRepository) Disable(
	ctx context.Context,
	workspaceID, email, endpointID string,
) error {
	if email == "" || endpointID == "" {
		return errors.New("contact endpoint email and endpoint_id are required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE contact_endpoints
		SET enabled = FALSE, version = version + 1, updated_at = CURRENT_TIMESTAMP
		WHERE endpoint_id = $1 AND email = $2 AND enabled
	`, endpointID, email); err != nil {
		return fmt.Errorf("disable contact endpoint %s: %w", endpointID, err)
	}
	return nil
}

func (r *ContactEndpointPostgresRepository) ListActiveByEmail(
	ctx context.Context,
	workspaceID, email, channel string,
) ([]*domain.ContactEndpoint, error) {
	if email == "" {
		return nil, errors.New("contact endpoint email is required")
	}
	if channel == "" {
		channel = domain.ChannelPush
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT endpoint_id, email, channel, provider, platform, address_ciphertext,
			locale, timezone, app_id, device_id, attributes, enabled, version,
			created_at, updated_at, last_seen_at
		FROM contact_endpoints
		WHERE email = $1 AND channel = $2 AND enabled
		ORDER BY last_seen_at DESC, endpoint_id
	`, email, channel)
	if err != nil {
		return nil, fmt.Errorf("list active contact endpoints: %w", err)
	}
	defer rows.Close()

	endpoints := make([]*domain.ContactEndpoint, 0)
	for rows.Next() {
		var endpoint domain.ContactEndpoint
		var ciphertext string
		var locale, timezone, appID, deviceID sql.NullString
		var attributesJSON []byte
		if err := rows.Scan(
			&endpoint.EndpointID, &endpoint.Email, &endpoint.Channel, &endpoint.Provider,
			&endpoint.Platform, &ciphertext, &locale, &timezone, &appID, &deviceID,
			&attributesJSON, &endpoint.Enabled, &endpoint.Version, &endpoint.CreatedAt,
			&endpoint.UpdatedAt, &endpoint.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan contact endpoint: %w", err)
		}
		endpoint.Address, err = pkgcrypto.DecryptFromHexString(ciphertext, r.secretKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt contact endpoint %s: %w", endpoint.EndpointID, err)
		}
		endpoint.Locale = locale.String
		endpoint.Timezone = timezone.String
		endpoint.AppID = appID.String
		endpoint.DeviceID = deviceID.String
		if err := json.Unmarshal(attributesJSON, &endpoint.Attributes); err != nil {
			return nil, fmt.Errorf("decode contact endpoint %s attributes: %w", endpoint.EndpointID, err)
		}
		endpoints = append(endpoints, &endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact endpoints: %w", err)
	}
	return endpoints, nil
}
