package repository

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
)

type ContactFilePostgresRepository struct{ workspaceRepo domain.WorkspaceRepository }

func NewContactFileRepository(workspaces domain.WorkspaceRepository) *ContactFilePostgresRepository {
	return &ContactFilePostgresRepository{workspaceRepo: workspaces}
}

func (r *ContactFilePostgresRepository) Fields(ctx context.Context, workspaceID string) ([]domain.ContactFileField, error) {
	workspace, err := r.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	fields := domain.BaseContactFileFields(workspace.Settings.CustomFieldLabels)
	known := map[string]bool{}
	for _, field := range fields {
		known[field.Key] = true
	}
	fields = append(fields, domain.ContactFileField{Key: "profile.status", Header: "profile.status", Type: "string"}, domain.ContactFileField{Key: "profile.tags", Header: "profile.tags", Type: "json"})
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	// Include configured fields even before any customer has a value, and
	// schemaless attributes created directly on customer profiles.
	rows, err := db.QueryContext(ctx, `WITH observed AS (
	 SELECT key, CASE WHEN count(DISTINCT jsonb_typeof(value)) = 1 THEN min(jsonb_typeof(value)) ELSE 'auto' END AS type
	 FROM (SELECT attributes FROM customer_profiles UNION ALL SELECT attributes FROM contact_profiles) profiles
	 CROSS JOIN LATERAL jsonb_each(profiles.attributes) GROUP BY key
	), definitions AS (
	 SELECT key, name, schema->>'type' AS type, settings->>'writable' AS writable, status
	 FROM business_definitions WHERE kind = 'customer_attribute'
	)
	SELECT COALESCE(d.key,o.key), COALESCE(d.name,o.key), COALESCE(d.type,o.type,'auto'), COALESCE(d.writable,'true') <> 'false'
	FROM observed o FULL JOIN definitions d ON d.key = o.key
	WHERE d.status IS DISTINCT FROM 'archived' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, name, kind string
		var writable bool
		if err := rows.Scan(&key, &name, &kind, &writable); err != nil {
			return nil, err
		}
		if known[key] {
			continue
		}
		if kind == "object" || kind == "array" {
			kind = "json"
		}
		if kind == "null" || kind == "" {
			kind = "auto"
		}
		fieldKey := "profile.attributes." + key
		fields = append(fields, domain.ContactFileField{Key: fieldKey, Header: name + " (" + fieldKey + ")", Type: kind, ReadOnly: !writable})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return domain.UniqueContactFileHeaders(fields), nil
}

func (r *ContactFilePostgresRepository) LoadProfiles(ctx context.Context, workspaceID string, contacts []*domain.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	byEmail := map[string]*domain.Contact{}
	for _, contact := range contacts {
		if contact != nil {
			byEmail[contact.Email] = contact
		}
	}
	emails := make([]string, 0, len(byEmail))
	for email := range byEmail {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT c.email, COALESCE(cp.status,lp.status,''),
	 COALESCE(lp.attributes,'{}'::jsonb) || COALESCE(cp.attributes,'{}'::jsonb),
	 CASE WHEN c.customer_id IS NOT NULL THEN COALESCE((SELECT jsonb_agg(tag ORDER BY tag) FROM customer_tags WHERE customer_id=c.customer_id),'[]'::jsonb)
	 ELSE COALESCE((SELECT jsonb_agg(tag ORDER BY tag) FROM contact_tags WHERE email=c.email),'[]'::jsonb) END
	 FROM contacts c LEFT JOIN customer_profiles cp ON cp.customer_id=c.customer_id
	 LEFT JOIN contact_profiles lp ON lp.email=c.email WHERE c.email=ANY($1::text[])`, pq.Array(emails))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var email, status string
		var attributes, tags []byte
		if err := rows.Scan(&email, &status, &attributes, &tags); err != nil {
			return err
		}
		profile := &domain.AudienceProfile{}
		if strings.TrimSpace(status) != "" {
			profile.Status = &status
		}
		if err := json.Unmarshal(attributes, &profile.Attributes); err != nil {
			return err
		}
		if err := json.Unmarshal(tags, &profile.Tags); err != nil {
			return err
		}
		if contact := byEmail[email]; contact != nil {
			contact.Profile = profile
		}
	}
	return rows.Err()
}
