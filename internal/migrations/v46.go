package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

// V46Migration introduces the workspace-local Customer authority model while
// preserving Contact as a compatibility projection during the transition.
type V46Migration struct{}

func (m *V46Migration) GetMajorVersion() float64  { return 46.0 }
func (m *V46Migration) HasSystemUpdate() bool     { return true }
func (m *V46Migration) HasWorkspaceUpdate() bool  { return true }
func (m *V46Migration) ShouldRestartServer() bool { return false }

func (m *V46Migration) UpdateSystem(ctx context.Context, _ *config.Config, db DBExecutor) error {
	for _, statement := range schema.WorkspaceSequenceMigrationStatements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v46: allocate permanent workspace sequences: %w", err)
		}
	}

	permissionStatements := []string{
		`UPDATE user_workspaces SET permissions = permissions || jsonb_build_object('customers', COALESCE(permissions->'contacts', '{"read":false,"write":false}'::jsonb)) WHERE permissions IS NOT NULL AND NOT permissions ? 'customers'`,
		`UPDATE workspace_invitations SET permissions = permissions || jsonb_build_object('customers', COALESCE(permissions->'contacts', '{"read":false,"write":false}'::jsonb)) WHERE permissions IS NOT NULL AND NOT permissions ? 'customers'`,
	}
	for _, statement := range permissionStatements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v46: copy contacts permissions to customers: %w", err)
		}
	}
	return nil
}

func (m *V46Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if workspace == nil || workspace.Sequence < domain.MinWorkspaceSequence || workspace.Sequence > domain.MaxWorkspaceSequence {
		return fmt.Errorf("v46: workspace sequence must be between %d and %d", domain.MinWorkspaceSequence, domain.MaxWorkspaceSequence)
	}
	if cfg == nil || strings.TrimSpace(cfg.Security.SecretKey) == "" {
		return errors.New("v46: customer identity encryption secret is required")
	}
	if db == nil {
		return errors.New("v46: workspace database is required")
	}

	for _, statement := range schema.CustomerTableDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v46: create customer schema for workspace %s: %w", workspace.ID, err)
		}
	}
	if err := rejectV46DuplicateContactKeys(ctx, workspace.ID, db); err != nil {
		return err
	}

	backfillStatements := []struct {
		query string
		args  []interface{}
	}{
		{query: `WITH customer_seeds AS (
			SELECT email, gen_random_uuid() AS customer_id, external_id, created_at, updated_at
			FROM contacts WHERE customer_id IS NULL
		), inserted_customers AS (
			INSERT INTO customers (id, customer_no, external_user_id, created_at, updated_at)
			SELECT customer_id,
				'U' || LPAD($1::text, 4, '0') ||
				TO_CHAR(created_at AT TIME ZONE 'Asia/Shanghai', 'YYYYMMDDHH24MISS') ||
				'08' || REPLACE(customer_id::text, '-', ''),
				NULLIF(BTRIM(external_id), ''), created_at, updated_at
			FROM customer_seeds
			ON CONFLICT DO NOTHING
			RETURNING id
		)
		UPDATE contacts c SET customer_id = seeds.customer_id
		FROM customer_seeds seeds
		WHERE c.email = seeds.email AND c.customer_id IS NULL`, args: []interface{}{workspace.Sequence}},
		{query: `INSERT INTO customer_profiles (
			customer_id, status, language, timezone, attributes, version, created_at, updated_at
		)
		SELECT c.customer_id, cp.status, c.language, c.timezone,
			COALESCE(cp.attributes, '{}'::jsonb) || jsonb_strip_nulls(jsonb_build_object(
				'first_name', c.first_name, 'last_name', c.last_name, 'full_name', c.full_name,
				'phone', c.phone, 'address_line_1', c.address_line_1, 'address_line_2', c.address_line_2,
				'country', c.country, 'postcode', c.postcode, 'state', c.state, 'job_title', c.job_title,
				'custom_string_1', c.custom_string_1, 'custom_string_2', c.custom_string_2,
				'custom_string_3', c.custom_string_3, 'custom_string_4', c.custom_string_4,
				'custom_string_5', c.custom_string_5, 'custom_number_1', c.custom_number_1,
				'custom_number_2', c.custom_number_2, 'custom_number_3', c.custom_number_3,
				'custom_number_4', c.custom_number_4, 'custom_number_5', c.custom_number_5,
				'custom_datetime_1', c.custom_datetime_1, 'custom_datetime_2', c.custom_datetime_2,
				'custom_datetime_3', c.custom_datetime_3, 'custom_datetime_4', c.custom_datetime_4,
				'custom_datetime_5', c.custom_datetime_5, 'custom_json_1', c.custom_json_1,
				'custom_json_2', c.custom_json_2, 'custom_json_3', c.custom_json_3,
				'custom_json_4', c.custom_json_4, 'custom_json_5', c.custom_json_5
			)), COALESCE(cp.version, 1), c.created_at, GREATEST(c.updated_at, COALESCE(cp.updated_at, c.updated_at))
		FROM contacts c LEFT JOIN contact_profiles cp ON cp.email = c.email
		WHERE c.customer_id IS NOT NULL
		ON CONFLICT (customer_id) DO NOTHING`},
		{query: `INSERT INTO customer_tags (customer_id, tag, created_at)
		SELECT c.customer_id, ct.tag, ct.created_at
		FROM contact_tags ct JOIN contacts c ON c.email = ct.email
		WHERE c.customer_id IS NOT NULL
		ON CONFLICT (customer_id, tag) DO NOTHING`},
		{query: `INSERT INTO customer_list_memberships (customer_id, list_id, status, created_at, updated_at)
		SELECT c.customer_id, cl.list_id, cl.status, cl.created_at, cl.updated_at
		FROM contact_lists cl JOIN contacts c ON c.email = cl.email
		WHERE c.customer_id IS NOT NULL AND cl.deleted_at IS NULL
		ON CONFLICT (customer_id, list_id) DO NOTHING`},
		{query: `UPDATE contact_endpoints ce SET customer_id = c.customer_id
		FROM contacts c WHERE ce.email = c.email AND ce.customer_id IS NULL`},
	}
	for _, statement := range backfillStatements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("v46: backfill customer authority for workspace %s: %w", workspace.ID, err)
		}
	}

	if err := backfillV46CustomerIdentities(ctx, cfg.Security.SecretKey, workspace.ID, db); err != nil {
		return err
	}
	return nil
}

func rejectV46DuplicateContactKeys(ctx context.Context, workspaceID string, db DBExecutor) error {
	checks := []struct {
		query string
		name  string
	}{
		{query: `SELECT external_id FROM contacts WHERE NULLIF(BTRIM(external_id), '') IS NOT NULL GROUP BY external_id HAVING COUNT(*) > 1 LIMIT 1`, name: "external user ID"},
		{query: `SELECT LOWER(BTRIM(email)) AS normalized_email FROM contacts GROUP BY LOWER(BTRIM(email)) HAVING COUNT(*) > 1 LIMIT 1`, name: "normalized email identity"},
	}
	for _, check := range checks {
		var duplicate string
		err := db.QueryRowContext(ctx, check.query).Scan(&duplicate)
		switch {
		case err == nil:
			return fmt.Errorf("v46: workspace %s has duplicate %s %q; resolve the collision before migration", workspaceID, check.name, duplicate)
		case errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return fmt.Errorf("v46: preflight %s uniqueness for workspace %s: %w", check.name, workspaceID, err)
		}
	}
	return nil
}

func backfillV46CustomerIdentities(ctx context.Context, secretKey, workspaceID string, db DBExecutor) error {
	rows, err := db.QueryContext(ctx, `SELECT c.customer_id, c.email, c.phone
		FROM contacts c
		WHERE c.customer_id IS NOT NULL
		ORDER BY c.email`)
	if err != nil {
		return fmt.Errorf("v46: read legacy identities for workspace %s: %w", workspaceID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var customerID string
		var email string
		var phone sql.NullString
		if err := rows.Scan(&customerID, &email, &phone); err != nil {
			return fmt.Errorf("v46: scan legacy identities for workspace %s: %w", workspaceID, err)
		}
		if err := insertV46Identity(ctx, db, secretKey, customerID, domain.CustomerIdentityEmail, email, true); err != nil {
			return fmt.Errorf("v46: migrate email identity for workspace %s: %w", workspaceID, err)
		}
		if phone.Valid && strings.TrimSpace(phone.String) != "" {
			if _, err := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: domain.CustomerIdentityPhone, Value: phone.String}); err == nil {
				if err := insertV46Identity(ctx, db, secretKey, customerID, domain.CustomerIdentityPhone, phone.String, true); err != nil {
					return fmt.Errorf("v46: migrate phone identity for workspace %s: %w", workspaceID, err)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("v46: iterate legacy identities for workspace %s: %w", workspaceID, err)
	}
	return nil
}

func insertV46Identity(ctx context.Context, db DBExecutor, secretKey, customerID string, identityType domain.CustomerIdentityType, value string, primary bool) error {
	normalized, err := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: identityType, Value: value})
	if err != nil {
		return err
	}
	ciphertext, err := pkgcrypto.EncryptString(normalized.Value, secretKey)
	if err != nil {
		return fmt.Errorf("encrypt identity: %w", err)
	}
	fingerprint, err := domain.CustomerIdentityFingerprint(secretKey, normalized)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO customer_identities (
		id, customer_id, identity_type, value_ciphertext, lookup_fingerprint, display_hint, is_primary
	) VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (identity_type, lookup_fingerprint) DO NOTHING`,
		uuid.New(), customerID, normalized.Type, ciphertext, fingerprint, normalized.DisplayHint, primary)
	return err
}

func init() { Register(&V46Migration{}) }
