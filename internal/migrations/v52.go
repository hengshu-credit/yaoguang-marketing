package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V52Migration lets campaigns use a list directly and records the lists
// selected when a customer import is staged.
type V52Migration struct{}

func (m *V52Migration) GetMajorVersion() float64                                       { return 52.0 }
func (m *V52Migration) HasSystemUpdate() bool                                          { return false }
func (m *V52Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V52Migration) ShouldRestartServer() bool                                      { return false }
func (m *V52Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

var v52WorkspaceStatements = []struct {
	name  string
	query string
}{
	{"make campaign audience optional", `ALTER TABLE campaign_versions ALTER COLUMN audience_id DROP NOT NULL`},
	{"make campaign audience version optional", `ALTER TABLE campaign_versions ALTER COLUMN audience_version DROP NOT NULL`},
	{"add campaign list source", `ALTER TABLE campaign_versions ADD COLUMN IF NOT EXISTS list_id VARCHAR(32)`},
	{"replace campaign source constraint", `ALTER TABLE campaign_versions DROP CONSTRAINT IF EXISTS campaign_versions_source_check`},
	{"validate campaign source", `ALTER TABLE campaign_versions ADD CONSTRAINT campaign_versions_source_check CHECK (
		(audience_id IS NOT NULL AND audience_version IS NOT NULL AND list_id IS NULL)
		OR (audience_id IS NULL AND audience_version IS NULL AND list_id IS NOT NULL)
	)`},
	{"add import list bindings", `ALTER TABLE import_jobs ADD COLUMN IF NOT EXISTS list_ids TEXT[] NOT NULL DEFAULT '{}'::text[]`},
}

func (m *V52Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v52: workspace database is required")
	}

	workspaceID := "unknown"
	if workspace != nil {
		workspaceID = workspace.ID
	}
	for _, statement := range v52WorkspaceStatements {
		if _, err := db.ExecContext(ctx, statement.query); err != nil {
			return fmt.Errorf("v52: %s for workspace %s: %w", statement.name, workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V52Migration{}) }
