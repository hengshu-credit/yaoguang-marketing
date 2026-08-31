package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V53Migration records the exact Audience version and candidate build resolved
// when a Campaign Run actually begins.
type V53Migration struct{}

func (m *V53Migration) GetMajorVersion() float64                                       { return 53.0 }
func (m *V53Migration) HasSystemUpdate() bool                                          { return false }
func (m *V53Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V53Migration) ShouldRestartServer() bool                                      { return false }
func (m *V53Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

var v53WorkspaceStatements = []struct {
	name  string
	query string
}{
	{"add campaign run audience", `ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_id UUID REFERENCES audiences(id) ON DELETE RESTRICT`},
	{"add campaign run audience version", `ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_version INTEGER`},
	{"add campaign run audience build", `ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_build_id UUID REFERENCES audience_builds(id) ON DELETE RESTRICT`},
	{"add campaign run audience version reference", `DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'campaign_runs_audience_version_fkey') THEN
			ALTER TABLE campaign_runs ADD CONSTRAINT campaign_runs_audience_version_fkey
				FOREIGN KEY (audience_id, audience_version)
				REFERENCES audience_versions(audience_id, version) ON DELETE RESTRICT;
		END IF;
	END $$`},
	{"index campaign run audience build", `CREATE INDEX IF NOT EXISTS idx_campaign_runs_audience_build ON campaign_runs(audience_build_id) WHERE audience_build_id IS NOT NULL`},
}

func (m *V53Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v53: workspace database is required")
	}
	workspaceID := "unknown"
	if workspace != nil {
		workspaceID = workspace.ID
	}
	for _, statement := range v53WorkspaceStatements {
		if _, err := db.ExecContext(ctx, statement.query); err != nil {
			return fmt.Errorf("v53: %s for workspace %s: %w", statement.name, workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V53Migration{}) }
