package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ConsoleThemeRepository struct{ db *sql.DB }

func NewConsoleThemeRepository(db *sql.DB) *ConsoleThemeRepository {
	return &ConsoleThemeRepository{db: db}
}
func (r *ConsoleThemeRepository) Set(ctx context.Context, workspaceID string, value *domain.ConsoleThemeSettings) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE workspaces SET settings = jsonb_set(COALESCE(settings,'{}'::jsonb), '{console_theme}', $1::jsonb, true), updated_at = CURRENT_TIMESTAMP WHERE id = $2`, data, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
