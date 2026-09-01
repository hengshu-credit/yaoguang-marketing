package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type TemplateCategoryRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

func NewTemplateCategoryRepository(workspaceRepo domain.WorkspaceRepository) *TemplateCategoryRepository {
	return &TemplateCategoryRepository{workspaceRepo: workspaceRepo}
}

const templateCategorySelectColumns = `c.id, c.name, c.purpose, c.sort_order, c.is_system, c.is_active,
	c.created_at, c.updated_at,
	(SELECT COUNT(DISTINCT t.id) FROM templates t WHERE t.category = c.id AND t.deleted_at IS NULL) AS usage_count`

func (r *TemplateCategoryRepository) List(ctx context.Context, workspaceID string, includeInactive bool) ([]domain.TemplateCategoryDefinition, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection for template categories: %w", err)
	}
	query := `SELECT ` + templateCategorySelectColumns + ` FROM template_categories c`
	if !includeInactive {
		query += ` WHERE c.is_active = TRUE`
	}
	query += ` ORDER BY c.sort_order ASC, c.name ASC, c.id ASC`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list template categories: %w", err)
	}
	defer rows.Close()
	result := []domain.TemplateCategoryDefinition{}
	for rows.Next() {
		category, scanErr := scanTemplateCategory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate template categories: %w", err)
	}
	return result, nil
}

type templateCategoryScanner interface {
	Scan(dest ...interface{}) error
}

func scanTemplateCategory(scanner templateCategoryScanner) (*domain.TemplateCategoryDefinition, error) {
	var category domain.TemplateCategoryDefinition
	if err := scanner.Scan(&category.ID, &category.Name, &category.Purpose, &category.SortOrder,
		&category.IsSystem, &category.IsActive, &category.CreatedAt, &category.UpdatedAt, &category.UsageCount); err != nil {
		return nil, fmt.Errorf("scan template category: %w", err)
	}
	return &category, nil
}

func (r *TemplateCategoryRepository) Get(ctx context.Context, workspaceID, id string) (*domain.TemplateCategoryDefinition, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection for template category: %w", err)
	}
	category, err := scanTemplateCategory(db.QueryRowContext(ctx,
		`SELECT `+templateCategorySelectColumns+` FROM template_categories c WHERE c.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) || (err != nil && errors.Is(errors.Unwrap(err), sql.ErrNoRows)) {
		return nil, domain.ErrTemplateCategoryNotFound
	}
	return category, err
}

func (r *TemplateCategoryRepository) Create(ctx context.Context, workspaceID string, category *domain.TemplateCategoryDefinition) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace connection for template category create: %w", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO template_categories
		(id, name, purpose, sort_order, is_system, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, category.ID, category.Name, category.Purpose,
		category.SortOrder, category.IsSystem, category.IsActive, category.CreatedAt, category.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create template category: %w", err)
	}
	return nil
}

func (r *TemplateCategoryRepository) Update(ctx context.Context, workspaceID string, category *domain.TemplateCategoryDefinition) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace connection for template category update: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE template_categories SET name = $2, sort_order = $3,
		is_active = $4, updated_at = $5 WHERE id = $1`, category.ID, category.Name, category.SortOrder,
		category.IsActive, category.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update template category: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return domain.ErrTemplateCategoryNotFound
	}
	return nil
}

func (r *TemplateCategoryRepository) Delete(ctx context.Context, workspaceID, id string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace connection for template category delete: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin template category delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var system bool
	if err := tx.QueryRowContext(ctx, `SELECT is_system FROM template_categories WHERE id = $1 FOR UPDATE`, id).Scan(&system); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrTemplateCategoryNotFound
		}
		return fmt.Errorf("lock template category: %w", err)
	}
	if system {
		return domain.ErrTemplateCategorySystem
	}
	var usage int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT id) FROM templates WHERE category = $1 AND deleted_at IS NULL`, id).Scan(&usage); err != nil {
		return fmt.Errorf("count template category usage: %w", err)
	}
	if usage > 0 {
		return domain.ErrTemplateCategoryInUse
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM template_categories WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete template category: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit template category delete: %w", err)
	}
	return nil
}

var _ domain.TemplateCategoryRepository = (*TemplateCategoryRepository)(nil)
