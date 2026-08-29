package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
)

type AudienceProfilePostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewAudienceProfileRepository(workspaceRepo domain.WorkspaceRepository) *AudienceProfilePostgresRepository {
	return &AudienceProfilePostgresRepository{workspaceRepo: workspaceRepo}
}

func NewAudienceProfileRepositoryWithDB(db *sql.DB) *AudienceProfilePostgresRepository {
	return &AudienceProfilePostgresRepository{db: db}
}

func (r *AudienceProfilePostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection: %w", err)
	}
	return db, nil
}

func (r *AudienceProfilePostgresRepository) EnsureContacts(
	ctx context.Context,
	workspaceID string,
	emails []string,
) error {
	if len(emails) == 0 {
		return nil
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO contacts (email)
		SELECT email FROM unnest($1::text[]) AS email
		ON CONFLICT (email) DO NOTHING
	`, pq.Array(emails)); err != nil {
		return fmt.Errorf("ensure ingest contacts: %w", err)
	}
	return nil
}

func (r *AudienceProfilePostgresRepository) UpsertProfile(
	ctx context.Context,
	workspaceID string,
	email string,
	status *string,
	attributes map[string]interface{},
) error {
	if email == "" || (status == nil && attributes == nil) {
		return nil
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	attributesJSON := []byte("{}")
	attributesProvided := attributes != nil
	if attributesProvided {
		attributesJSON, err = json.Marshal(attributes)
		if err != nil {
			return fmt.Errorf("marshal profile attributes: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO contact_profiles (email, status, attributes)
		VALUES ($1, $2, $3)
		ON CONFLICT (email) DO UPDATE SET
			status = COALESCE(EXCLUDED.status, contact_profiles.status),
			attributes = CASE WHEN $4::boolean
				THEN contact_profiles.attributes || EXCLUDED.attributes
				ELSE contact_profiles.attributes
			END,
			version = contact_profiles.version + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE ($2::text IS NOT NULL AND EXCLUDED.status IS DISTINCT FROM contact_profiles.status)
		   OR ($4::boolean AND NOT contact_profiles.attributes @> EXCLUDED.attributes)
	`, email, status, string(attributesJSON), attributesProvided); err != nil {
		return fmt.Errorf("upsert contact profile: %w", err)
	}
	return nil
}

func (r *AudienceProfilePostgresRepository) ApplyTags(
	ctx context.Context,
	workspaceID string,
	email string,
	operation string,
	tags []string,
) ([]string, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tag mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	switch operation {
	case domain.TagOperationSet:
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM contact_tags
			WHERE email = $1
			  AND (cardinality($2::text[]) = 0 OR NOT (tag = ANY($2::text[])))
		`, email, pq.Array(tags)); err != nil {
			return nil, fmt.Errorf("remove replaced tags: %w", err)
		}
		if err := insertContactTags(ctx, tx, email, tags); err != nil {
			return nil, err
		}
	case domain.TagOperationAdd:
		if err := insertContactTags(ctx, tx, email, tags); err != nil {
			return nil, err
		}
	case domain.TagOperationRemove:
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM contact_tags WHERE email = $1 AND tag = ANY($2::text[])
		`, email, pq.Array(tags)); err != nil {
			return nil, fmt.Errorf("remove contact tags: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid tag operation %q", operation)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT tag FROM contact_tags WHERE email = $1 ORDER BY tag
	`, email)
	if err != nil {
		return nil, fmt.Errorf("list contact tags: %w", err)
	}
	current := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan contact tag: %w", err)
		}
		current = append(current, tag)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close contact tag rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact tags: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tag mutation: %w", err)
	}
	return current, nil
}

func (r *AudienceProfilePostgresRepository) GetProfiles(
	ctx context.Context,
	workspaceID string,
	emails []string,
) (map[string]*domain.AudienceProfile, error) {
	profiles := make(map[string]*domain.AudienceProfile)
	if len(emails) == 0 {
		return profiles, nil
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT requested.email,
		       cp.status,
		       COALESCE(cp.attributes, '{}'::jsonb),
		       COALESCE(
		           array_agg(ct.tag ORDER BY ct.tag) FILTER (WHERE ct.tag IS NOT NULL),
		           ARRAY[]::text[]
		       )
		FROM unnest($1::text[]) AS requested(email)
		LEFT JOIN contact_profiles cp ON cp.email = requested.email
		LEFT JOIN contact_tags ct ON ct.email = requested.email
		GROUP BY requested.email, cp.status, cp.attributes
	`, pq.Array(emails))
	if err != nil {
		return nil, fmt.Errorf("get audience profiles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var email string
		var status sql.NullString
		var attributesJSON []byte
		var tags []string
		if err := rows.Scan(&email, &status, &attributesJSON, pq.Array(&tags)); err != nil {
			return nil, fmt.Errorf("scan audience profile: %w", err)
		}
		attributes := make(map[string]interface{})
		if err := json.Unmarshal(attributesJSON, &attributes); err != nil {
			return nil, fmt.Errorf("decode audience profile %s: %w", email, err)
		}
		if !status.Valid && len(attributes) == 0 && len(tags) == 0 {
			continue
		}
		profile := &domain.AudienceProfile{Attributes: attributes, Tags: tags}
		if status.Valid {
			profile.Status = &status.String
		}
		profiles[email] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audience profiles: %w", err)
	}
	return profiles, nil
}

func insertContactTags(ctx context.Context, tx *sql.Tx, email string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO contact_tags (email, tag)
		SELECT $1, tag FROM unnest($2::text[]) AS tag
		ON CONFLICT (email, tag) DO NOTHING
	`, email, pq.Array(tags)); err != nil {
		return fmt.Errorf("add contact tags: %w", err)
	}
	return nil
}
