package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V43Migration adds first-class SMS and push content to versioned templates.
type V43Migration struct{}

func (m *V43Migration) GetMajorVersion() float64                                       { return 43.0 }
func (m *V43Migration) HasSystemUpdate() bool                                          { return false }
func (m *V43Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V43Migration) ShouldRestartServer() bool                                      { return false }
func (m *V43Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V43Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}
	if _, err := db.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("v43: failed to set lock timeout for workspace %s: %w", workspaceID, err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE templates ADD COLUMN IF NOT EXISTS sms JSONB`); err != nil {
		return fmt.Errorf("v43: failed to add sms template column for workspace %s: %w", workspaceID, err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE templates ADD COLUMN IF NOT EXISTS push JSONB`); err != nil {
		return fmt.Errorf("v43: failed to add push template column for workspace %s: %w", workspaceID, err)
	}
	return nil
}

func init() { Register(&V43Migration{}) }
