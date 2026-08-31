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
	workspaceRepo     domain.WorkspaceRepository
	db                *sql.DB
	conditionCompiler AudienceConditionCompiler
	now               func() time.Time
}

type AudienceConditionCompiler func(*domain.TreeNode, int) (string, []interface{}, error)

func NewAudienceRepository(workspaceRepo domain.WorkspaceRepository) *AudiencePostgresRepository {
	return &AudiencePostgresRepository{workspaceRepo: workspaceRepo}
}
func NewAudienceRepositoryWithDB(db *sql.DB) *AudiencePostgresRepository {
	return &AudiencePostgresRepository{db: db}
}

func (r *AudiencePostgresRepository) SetConditionCompiler(compiler AudienceConditionCompiler) {
	r.conditionCompiler = compiler
}

func (r *AudiencePostgresRepository) currentTime() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
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

func (r *AudiencePostgresRepository) ListAudiences(ctx context.Context, workspaceID string, limit, offset int) ([]domain.Audience, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audiences WHERE status = 'active'`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name, description, kind, active_version, active_build_id, created_at, updated_at
		FROM audiences WHERE status = 'active' ORDER BY updated_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Audience, 0)
	for rows.Next() {
		var item domain.Audience
		var description, buildID sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &description, &item.Kind, &item.ActiveVersion, &buildID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		item.Description, item.ActiveBuildID = description.String, buildID.String
		items = append(items, item)
	}
	return items, total, rows.Err()
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
	args              []interface{}
	offset            int
	conditionCompiler AudienceConditionCompiler
}

func (c *audienceSQLCompiler) compile(expression domain.AudienceExpression) (string, error) {
	if err := expression.Validate(); err != nil {
		return "", err
	}
	if expression.Condition != nil {
		if c.conditionCompiler == nil {
			return "", errors.New("audience condition compiler is required")
		}
		query, args, err := c.conditionCompiler(expression.Condition, c.offset+len(c.args))
		if err != nil {
			return "", fmt.Errorf("compile audience condition: %w", err)
		}
		c.args = append(c.args, args...)
		return query, nil
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
	return compileAudienceExpressionWithConditionCompiler(expression, 0, nil)
}

func compileAudienceExpressionWithConditionCompiler(expression domain.AudienceExpression, offset int, conditionCompiler AudienceConditionCompiler) (string, []interface{}, error) {
	compiler := &audienceSQLCompiler{offset: offset, conditionCompiler: conditionCompiler}
	query, err := compiler.compile(expression)
	return query, compiler.args, err
}

func compileAudienceExpressionWithOffset(expression domain.AudienceExpression, offset int) (string, []interface{}, error) {
	return compileAudienceExpressionWithConditionCompiler(expression, offset, nil)
}

func (r *AudiencePostgresRepository) compileAudienceExpression(expression domain.AudienceExpression, offset int) (string, []interface{}, error) {
	return compileAudienceExpressionWithConditionCompiler(expression, offset, r.conditionCompiler)
}

func (r *AudiencePostgresRepository) PreviewAudience(ctx context.Context, workspaceID string, expression domain.AudienceExpression, limit int) ([]domain.CustomerSummary, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	compiled, args, err := r.compileAudienceExpression(expression, 0)
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
	compiled, args, err := r.compileAudienceExpression(item.Definition, 1)
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

// BuildAudienceSnapshot materializes one immutable, run-scoped candidate set.
// Unlike the legacy BuildAudience endpoint, it deliberately does not update
// audiences.active_build_id: a marketing run persists this exact build ID and
// no later build is allowed to replace its source.
func (r *AudiencePostgresRepository) BuildAudienceSnapshot(ctx context.Context, workspaceID, audienceID string, version int) (*domain.AudienceBuild, error) {
	item, err := r.GetAudienceVersion(ctx, workspaceID, audienceID, version)
	if err != nil {
		return nil, err
	}
	compiled, args, err := r.compileAudienceExpression(item.Definition, 1)
	if err != nil {
		return nil, err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := r.currentTime()
	build := &domain.AudienceBuild{
		ID:              uuid.New().String(),
		AudienceID:      audienceID,
		AudienceVersion: version,
		Status:          "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO audience_builds (
		id, audience_id, audience_version, status, created_at, updated_at
	) VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, 'pending', $4, $4)`,
		build.ID, audienceID, version, now); err != nil {
		return nil, fmt.Errorf("create audience snapshot build: %w", err)
	}

	failBuild := func(cause error) (*domain.AudienceBuild, error) {
		build.Status = "failed"
		build.ErrorDetail = cause.Error()
		build.UpdatedAt = r.currentTime()
		_, _ = db.ExecContext(context.Background(), `UPDATE audience_builds SET status = 'failed',
			error_detail = $2, updated_at = $3 WHERE id = NULLIF($1, '')::uuid`,
			build.ID, build.ErrorDetail, build.UpdatedAt)
		return build, cause
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return failBuild(fmt.Errorf("begin audience snapshot: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE audience_builds SET status = 'building',
		started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND status = 'pending'`, build.ID); err != nil {
		return failBuild(fmt.Errorf("start audience snapshot: %w", err))
	}
	insertSQL := `INSERT INTO audience_memberships (build_id, customer_id, ordinal)
		SELECT NULLIF($1, '')::uuid, result.customer_id, ROW_NUMBER() OVER (ORDER BY result.customer_id)
		FROM (` + compiled + `) result
		JOIN customers customer ON customer.id = result.customer_id
		WHERE customer.merged_into_id IS NULL
		ON CONFLICT DO NOTHING`
	result, err := tx.ExecContext(ctx, insertSQL, append([]interface{}{build.ID}, args...)...)
	if err != nil {
		return failBuild(fmt.Errorf("materialize audience snapshot: %w", err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return failBuild(fmt.Errorf("count audience snapshot members: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `UPDATE audience_builds SET status = 'completed', member_count = $2,
		completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND status = 'building'`, build.ID, count); err != nil {
		return failBuild(fmt.Errorf("complete audience snapshot: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return failBuild(fmt.Errorf("commit audience snapshot: %w", err))
	}
	build.Status = "completed"
	build.MemberCount = count
	build.UpdatedAt = r.currentTime()
	return build, nil
}

// MatchesAudienceCustomer re-evaluates an immutable Audience version against
// current customer facts. False is a business result; database/compiler errors
// remain errors so callers can retry instead of silently suppressing a touch.
func (r *AudiencePostgresRepository) MatchesAudienceCustomer(ctx context.Context, workspaceID, audienceID string, version int, customerID string) (bool, error) {
	if strings.TrimSpace(customerID) == "" {
		return false, errors.New("customer id is required")
	}
	item, err := r.GetAudienceVersion(ctx, workspaceID, audienceID, version)
	if err != nil {
		return false, err
	}
	compiled, args, err := r.compileAudienceExpression(item.Definition, 0)
	if err != nil {
		return false, err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	args = append(args, customerID)
	query := `SELECT EXISTS (SELECT 1 FROM (` + compiled + `) audience_result
		WHERE audience_result.customer_id = NULLIF($` + fmt.Sprint(len(args)) + `, '')::uuid)`
	var matches bool
	if err := db.QueryRowContext(ctx, query, args...).Scan(&matches); err != nil {
		return false, fmt.Errorf("evaluate audience customer eligibility: %w", err)
	}
	return matches, nil
}

func (r *AudiencePostgresRepository) StartAudienceBuild(ctx context.Context, workspaceID, audienceID string, version int) (*domain.AudienceBuild, error) {
	if _, err := r.GetAudienceVersion(ctx, workspaceID, audienceID, version); err != nil {
		return nil, err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	build := &domain.AudienceBuild{ID: uuid.New().String(), AudienceID: audienceID, AudienceVersion: version,
		Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_, err = db.ExecContext(ctx, `INSERT INTO audience_builds (
		id, audience_id, audience_version, status, created_at, updated_at
	) VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, 'pending', $4, $4)`,
		build.ID, audienceID, version, build.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("start audience build: %w", err)
	}
	return build, nil
}

// ProcessAudienceBuildChunk materializes one keyset page in a single
// transaction. The build row is locked before reading its cursor, making a
// crash either commit both memberships and checkpoint or neither.
func (r *AudiencePostgresRepository) ProcessAudienceBuildChunk(ctx context.Context, workspaceID, buildID string, chunkSize int) (*domain.AudienceBuild, bool, error) {
	if chunkSize <= 0 || chunkSize > 20_000 {
		chunkSize = 5_000
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	build := &domain.AudienceBuild{ID: buildID}
	var lastCustomerID, errorDetail sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT audience_id, audience_version, status, last_customer_id,
		member_count, error_detail, created_at, updated_at FROM audience_builds
		WHERE id = NULLIF($1, '')::uuid FOR UPDATE`, buildID).Scan(&build.AudienceID,
		&build.AudienceVersion, &build.Status, &lastCustomerID, &build.MemberCount,
		&errorDetail, &build.CreatedAt, &build.UpdatedAt)
	if err != nil {
		return nil, false, err
	}
	build.LastCustomerID, build.ErrorDetail = lastCustomerID.String, errorDetail.String
	if build.Status == "completed" {
		return build, true, tx.Commit()
	}
	if build.Status == "failed" || build.Status == "cancelled" {
		return build, false, fmt.Errorf("audience build is %s", build.Status)
	}
	version := &domain.AudienceVersion{}
	var definition []byte
	err = tx.QueryRowContext(ctx, `SELECT definition FROM audience_versions
		WHERE audience_id = $1 AND version = $2`, build.AudienceID, build.AudienceVersion).Scan(&definition)
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(definition, &version.Definition); err != nil {
		return nil, false, err
	}
	compiled, args, err := r.compileAudienceExpression(version.Definition, 4)
	if err != nil {
		return nil, false, err
	}
	queryArgs := []interface{}{buildID, build.MemberCount, build.LastCustomerID, chunkSize}
	queryArgs = append(queryArgs, args...)
	var pageCount int64
	var pageLast sql.NullString
	query := `WITH page AS (
		SELECT DISTINCT result.customer_id FROM (` + compiled + `) result
		JOIN customers customer ON customer.id = result.customer_id
		WHERE customer.merged_into_id IS NULL
			AND result.customer_id > COALESCE(NULLIF($3, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
			ORDER BY result.customer_id LIMIT $4::integer
	), inserted AS (
		INSERT INTO audience_memberships (build_id, customer_id, ordinal)
		SELECT NULLIF($1, '')::uuid, customer_id, $2::bigint + ROW_NUMBER() OVER (ORDER BY customer_id)
		FROM page ON CONFLICT DO NOTHING RETURNING customer_id
	) SELECT COUNT(*), (SELECT customer_id::text FROM inserted ORDER BY customer_id DESC LIMIT 1) FROM inserted`
	if err := tx.QueryRowContext(ctx, query, queryArgs...).Scan(&pageCount, &pageLast); err != nil {
		return nil, false, err
	}
	build.MemberCount += pageCount
	if pageLast.Valid {
		build.LastCustomerID = pageLast.String
	}
	completed := pageCount < int64(chunkSize)
	status := "building"
	if completed {
		status = "completed"
	}
	_, err = tx.ExecContext(ctx, `UPDATE audience_builds SET status = $2::text, last_customer_id = NULLIF($3::text, '')::uuid,
		member_count = $4::bigint, started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
		completed_at = CASE WHEN $2::text = 'completed' THEN CURRENT_TIMESTAMP ELSE completed_at END,
		updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid`, buildID, status, build.LastCustomerID, build.MemberCount)
	if err != nil {
		return nil, false, err
	}
	if completed {
		if _, err := tx.ExecContext(ctx, `UPDATE audiences SET active_build_id = NULLIF($2, '')::uuid,
			updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND active_version = $3`, build.AudienceID, buildID, build.AudienceVersion); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	build.Status = status
	build.UpdatedAt = time.Now().UTC()
	return build, completed, nil
}

func (r *AudiencePostgresRepository) GetAudienceBuild(ctx context.Context, workspaceID, buildID string) (*domain.AudienceBuild, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	item := &domain.AudienceBuild{}
	var lastCustomerID, errorDetail sql.NullString
	err = db.QueryRowContext(ctx, `SELECT id, audience_id, audience_version, status, last_customer_id,
		member_count, error_detail, created_at, updated_at FROM audience_builds WHERE id = NULLIF($1, '')::uuid`, buildID).
		Scan(&item.ID, &item.AudienceID, &item.AudienceVersion, &item.Status, &lastCustomerID,
			&item.MemberCount, &errorDetail, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	item.LastCustomerID, item.ErrorDetail = lastCustomerID.String, errorDetail.String
	return item, nil
}

func (r *AudiencePostgresRepository) ListAudienceMembers(ctx context.Context, workspaceID, buildID, after string, limit int) ([]domain.CustomerSummary, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, "", err
	}
	rows, err := db.QueryContext(ctx, `SELECT customer.id, customer.customer_no, customer.external_user_id,
		customer.merged_into_id, customer.version, customer.created_at, customer.updated_at
		FROM audience_memberships membership JOIN customers customer ON customer.id = membership.customer_id
		WHERE membership.build_id = NULLIF($1, '')::uuid AND ($2 = '' OR customer.id > NULLIF($2, '')::uuid)
		ORDER BY customer.id LIMIT $3`, buildID, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.CustomerSummary, 0, limit+1)
	for rows.Next() {
		var item domain.CustomerSummary
		if err := rows.Scan(&item.ID, &item.CustomerNo, &item.ExternalUserID, &item.MergedIntoID, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, nil
}

func (r *AudiencePostgresRepository) ArchiveAudience(ctx context.Context, workspaceID, audienceID string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	var dependentCount int
	pattern := `%"leaf_type":"audience","ref_id":"` + audienceID + `"%`
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM audiences dependency JOIN audience_versions version
		 ON version.audience_id = dependency.id AND version.version = dependency.active_version
		 WHERE dependency.status = 'active' AND dependency.id <> NULLIF($1, '')::uuid AND version.definition::text LIKE $2)
		+ (SELECT COUNT(*) FROM campaign_versions WHERE audience_id = NULLIF($1, '')::uuid)`, audienceID, pattern).Scan(&dependentCount); err != nil {
		return err
	}
	if dependentCount > 0 {
		return errors.New("audience is referenced by another audience or campaign")
	}
	result, err := db.ExecContext(ctx, `UPDATE audiences SET status = 'archived', updated_at = CURRENT_TIMESTAMP
		WHERE id = NULLIF($1, '')::uuid AND status = 'active'`, audienceID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
