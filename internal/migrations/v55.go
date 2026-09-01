package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type V55Migration struct{}

func (m *V55Migration) GetMajorVersion() float64                                       { return 55.0 }
func (m *V55Migration) HasSystemUpdate() bool                                          { return false }
func (m *V55Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V55Migration) ShouldRestartServer() bool                                      { return false }
func (m *V55Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

var v55WorkspaceStatements = []struct {
	name  string
	query string
}{
	{"create template category catalogue", `CREATE TABLE IF NOT EXISTS template_categories (
		id VARCHAR(20) PRIMARY KEY,
		name VARCHAR(64) NOT NULL,
		purpose VARCHAR(20) NOT NULL CHECK (purpose IN ('marketing', 'transactional')),
		sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 10000),
		is_system BOOLEAN NOT NULL DEFAULT FALSE,
		is_active BOOLEAN NOT NULL DEFAULT TRUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{"seed built-in template categories", `INSERT INTO template_categories (id, name, purpose, sort_order, is_system, is_active) VALUES
		('marketing', 'Marketing', 'marketing', 10, TRUE, TRUE),
		('transactional', 'Transactional', 'transactional', 20, TRUE, TRUE),
		('welcome', 'Welcome', 'transactional', 30, TRUE, TRUE),
		('opt_in', 'Opt-in', 'transactional', 40, TRUE, TRUE),
		('unsubscribe', 'Unsubscribe', 'transactional', 50, TRUE, TRUE),
		('bounce', 'Bounce', 'transactional', 60, TRUE, TRUE),
		('blocklist', 'Blocklist', 'transactional', 70, TRUE, TRUE),
		('blog', 'Blog', 'marketing', 80, TRUE, TRUE),
		('other', 'Other', 'transactional', 90, TRUE, TRUE)
	ON CONFLICT (id) DO NOTHING`},
	{"register legacy template categories", `INSERT INTO template_categories (id, name, purpose, sort_order, is_system, is_active)
	SELECT DISTINCT category, INITCAP(REPLACE(category, '_', ' ')), 'transactional', 1000, FALSE, TRUE
	FROM templates
	WHERE category <> '' AND category ~ '^[a-z0-9]+([_-][a-z0-9]+)*$' AND LENGTH(category) <= 20
	ON CONFLICT (id) DO NOTHING`},
	{"index active template categories", `CREATE INDEX IF NOT EXISTS idx_template_categories_active_order ON template_categories(is_active, sort_order, id)`},
}

func (m *V55Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v55: workspace database is required")
	}
	workspaceID := "unknown"
	if workspace != nil {
		workspaceID = workspace.ID
	}
	for _, statement := range v55WorkspaceStatements {
		if _, err := db.ExecContext(ctx, statement.query); err != nil {
			return fmt.Errorf("v55: %s for workspace %s: %w", statement.name, workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V55Migration{}) }
