package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V41Migration adds the external audience profile and tag ingestion model.
type V41Migration struct{}

func (m *V41Migration) GetMajorVersion() float64 { return 41.0 }

func (m *V41Migration) HasSystemUpdate() bool { return false }

func (m *V41Migration) HasWorkspaceUpdate() bool { return true }

func (m *V41Migration) ShouldRestartServer() bool { return false }

func (m *V41Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V41Migration) UpdateWorkspace(
	ctx context.Context,
	_ *config.Config,
	workspace *domain.Workspace,
	db DBExecutor,
) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}
	if _, err := db.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("v41: failed to set lock timeout for workspace %s: %w", workspaceID, err)
	}
	if _, err := db.ExecContext(ctx, strings.Join(schema.AudienceIngestTableDefinitions(), ";\n")); err != nil {
		return fmt.Errorf("v41: failed to install audience ingest schema for workspace %s: %w", workspaceID, err)
	}
	return nil
}

func init() {
	Register(&V41Migration{})
}
