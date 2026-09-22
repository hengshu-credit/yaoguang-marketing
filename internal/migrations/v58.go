package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V58Migration expands the import job status constraint for the actionable
// needs_attention/retrying states introduced by the import retry workflow.
// The CREATE TABLE IF NOT EXISTS definition only affects new workspaces, so
// existing workspace databases need this explicit constraint migration.
type V58Migration struct{}

func (m *V58Migration) GetMajorVersion() float64                                       { return 58.0 }
func (m *V58Migration) HasSystemUpdate() bool                                          { return false }
func (m *V58Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V58Migration) ShouldRestartServer() bool                                      { return false }
func (m *V58Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V58Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, _ *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v58: workspace database is required")
	}
	statements := []string{
		`ALTER TABLE import_jobs DROP CONSTRAINT IF EXISTS import_jobs_status_check`,
		`ALTER TABLE import_jobs ADD CONSTRAINT import_jobs_status_check CHECK (status IN ('uploading', 'staged', 'processing', 'completed', 'needs_attention', 'retrying', 'rejected', 'cancelled'))`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v58: expand import job statuses: %w", err)
		}
	}
	return nil
}

func init() { Register(&V58Migration{}) }
