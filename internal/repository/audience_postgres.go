package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type AudiencePostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewAudienceRepository(workspaceRepo domain.WorkspaceRepository) *AudiencePostgresRepository {
	return &AudiencePostgresRepository{workspaceRepo: workspaceRepo}
}
func NewAudienceRepositoryWithDB(db *sql.DB) *AudiencePostgresRepository {
	return &AudiencePostgresRepository{db: db}
}
func (r *AudiencePostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

func (r *AudiencePostgresRepository) CreateAudience(ctx context.Context, workspaceID string, audience domain.Audience, version domain.AudienceVersion) error {
	if strings.TrimSpace(audience.ID) == "" {
		audience.ID = uuid.New().String()
	}
	if audience.ActiveVersion <= 0 {
		audience.ActiveVersion = 1
	}
	if version.Version <= 0 {
		version.Version = audience.ActiveVersion
	}
	version.AudienceID = audience.ID
	definition, err := version.Definition.CanonicalJSON()
	if err != nil {
		return err
	}
	version.DefinitionHash, err = version.Definition.VersionHash()
	if err != nil {
		return err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO audiences (id, name, description, kind, status, active_version, created_at, updated_at)
		VALUES (NULLIF($1, '')::uuid, $2, NULLIF($3, ''), $4, 'active', $5, $6, $6)`,
		audience.ID, audience.Name, audience.Description, audience.Kind, audience.ActiveVersion, audience.CreatedAt); err != nil {
		return fmt.Errorf("create audience: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audience_versions (audience_id, version, definition, definition_hash, created_at)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, $5)`, audience.ID, version.Version, definition, version.DefinitionHash, version.CreatedAt); err != nil {
		return fmt.Errorf("create audience version: %w", err)
	}
	return tx.Commit()
}

func (r *AudiencePostgresRepository) GetAudience(ctx context.Context, workspaceID, audienceID string) (*domain.Audience, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	audience := &domain.Audience{}
	var description, buildID sql.NullString
	err = db.QueryRowContext(ctx, `SELECT id, name, description, kind, active_version, active_build_id, created_at, updated_at
		FROM audiences WHERE id = NULLIF($1, '')::uuid AND status = 'active'`, audienceID).Scan(&audience.ID, &audience.Name,
		&description, &audience.Kind, &audience.ActiveVersion, &buildID, &audience.CreatedAt, &audience.UpdatedAt)
	if err != nil {
		return nil, err
	}
	audience.Description, audience.ActiveBuildID = description.String, buildID.String
	return audience, nil
}

func (r *AudiencePostgresRepository) GetAudienceVersion(ctx context.Context, workspaceID, audienceID string, version int) (*domain.AudienceVersion, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	item := &domain.AudienceVersion{}
	var definition []byte
	err = db.QueryRowContext(ctx, `SELECT audience_id, version, definition, definition_hash, created_at
		FROM audience_versions WHERE audience_id = NULLIF($1, '')::uuid AND version = $2`, audienceID, version).
		Scan(&item.AudienceID, &item.Version, &definition, &item.DefinitionHash, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(definition, &item.Definition); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *AudiencePostgresRepository) SaveAudienceVersion(ctx context.Context, workspaceID, audienceID string, expression domain.AudienceExpression) (*domain.AudienceVersion, error) {
	definition, err := expression.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	hash, _ := expression.VersionHash()
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT active_version + 1 FROM audiences WHERE id = NULLIF($1, '')::uuid AND status = 'active' FOR UPDATE`, audienceID).Scan(&version); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO audience_versions (audience_id, version, definition, definition_hash, created_at)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, $5)`, audienceID, version, definition, hash, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE audiences SET active_version = $2, active_build_id = NULL, updated_at = $3 WHERE id = NULLIF($1, '')::uuid`, audienceID, version, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &domain.AudienceVersion{AudienceID: audienceID, Version: version, Definition: expression, DefinitionHash: hash, CreatedAt: now}, nil
}

type audienceSQLCompiler struct {
	args   []interface{}
	offset int
}

func (c *audienceSQLCompiler) compile(expression domain.AudienceExpression) (string, error) {
	if err := expression.Validate(); err != nil {
		return "", err
	}
	if expression.LeafType != "" {
		c.args = append(c.args, expression.RefID)
		placeholder := fmt.Sprintf("$%d", c.offset+len(c.args))
		switch expression.LeafType {
		case domain.AudienceLeafList:
			return `SELECT customer_id FROM customer_list_memberships WHERE list_id = ` + placeholder + ` AND status = 'active'`, nil
		case domain.AudienceLeafSegment:
			return `SELECT customer_id FROM contact_segments WHERE segment_id = ` + placeholder + ` AND customer_id IS NOT NULL`, nil
		case domain.AudienceLeafAudience:
			return `SELECT membership.customer_id FROM audience_memberships membership JOIN audiences audience ON audience.active_build_id = membership.build_id WHERE audience.id = NULLIF(` + placeholder + `, '')::uuid`, nil
		}
	}
	compiled := make([]string, 0, len(expression.Children))
	for _, child := range expression.Children {
		query, err := c.compile(child)
		if err != nil {
			return "", err
		}
		compiled = append(compiled, "("+query+")")
	}
	separator := " UNION "
	if expression.Operator == domain.AudienceOperatorIntersection {
		separator = " INTERSECT "
	} else if expression.Operator == domain.AudienceOperatorExclusion {
		separator = " EXCEPT "
	}
	return strings.Join(compiled, separator), nil
}

func compileAudienceExpression(expression domain.AudienceExpression) (string, []interface{}, error) {
	compiler := &audienceSQLCompiler{}
	query, err := compiler.compile(expression)
	return query, compiler.args, err
}

func compileAudienceExpressionWithOffset(expression domain.AudienceExpression, offset int) (string, []interface{}, error) {
	compiler := &audienceSQLCompiler{offset: offset}
	query, err := compiler.compile(expression)
	return query, compiler.args, err
}

func (r *AudiencePostgresRepository) PreviewAudience(ctx context.Context, workspaceID string, expression domain.AudienceExpression, limit int) ([]domain.CustomerSummary, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	compiled, args, err := compileAudienceExpression(expression)
	if err != nil {
		return nil, 0, err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+compiled+`) audience_result`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, `SELECT customer.id, customer.customer_no, customer.external_user_id, customer.merged_into_id,
		customer.version, customer.created_at, customer.updated_at FROM customers customer
		JOIN (`+compiled+`) audience_result ON audience_result.customer_id = customer.id
		WHERE customer.merged_into_id IS NULL ORDER BY customer.id LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []domain.CustomerSummary{}
	for rows.Next() {
		var item domain.CustomerSummary
		if err := rows.Scan(&item.ID, &item.CustomerNo, &item.ExternalUserID, &item.MergedIntoID, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *AudiencePostgresRepository) BuildAudience(ctx context.Context, workspaceID, audienceID string, version int) (string, int64, error) {
	item, err := r.GetAudienceVersion(ctx, workspaceID, audienceID, version)
	if err != nil {
		return "", 0, err
	}
	compiled, args, err := compileAudienceExpressionWithOffset(item.Definition, 1)
	if err != nil {
		return "", 0, err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return "", 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback() }()
	buildID := uuid.New().String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO audience_builds (id, audience_id, audience_version, status, started_at)
		VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, 'building', CURRENT_TIMESTAMP)`, buildID, audienceID, version); err != nil {
		return "", 0, err
	}
	insertSQL := `INSERT INTO audience_memberships (build_id, customer_id, ordinal)
		SELECT NULLIF($1, '')::uuid, result.customer_id, ROW_NUMBER() OVER (ORDER BY result.customer_id)
		FROM (` + compiled + `) result JOIN customers customer ON customer.id = result.customer_id
		WHERE customer.merged_into_id IS NULL ON CONFLICT DO NOTHING`
	result, err := tx.ExecContext(ctx, insertSQL, append([]interface{}{buildID}, args...)...)
	if err != nil {
		return "", 0, err
	}
	count, _ := result.RowsAffected()
	if _, err := tx.ExecContext(ctx, `UPDATE audience_builds SET status = 'completed', member_count = $2,
		completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid`, buildID, count); err != nil {
		return "", 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE audiences SET active_build_id = NULLIF($2, '')::uuid, updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND active_version = $3`, audienceID, buildID, version); err != nil {
		return "", 0, err
	}
	if err := tx.Commit(); err != nil {
		return "", 0, err
	}
	return buildID, count, nil
}
