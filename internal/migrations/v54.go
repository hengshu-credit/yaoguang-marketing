package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type V54Migration struct{}

func (m *V54Migration) GetMajorVersion() float64                                       { return 54.0 }
func (m *V54Migration) HasSystemUpdate() bool                                          { return false }
func (m *V54Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V54Migration) ShouldRestartServer() bool                                      { return false }
func (m *V54Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

var v54WorkspaceStatements = []struct {
	name  string
	query string
}{
	{"add generic template content", `ALTER TABLE templates ADD COLUMN IF NOT EXISTS content JSONB`},
	{"add generic template schema version", `ALTER TABLE templates ADD COLUMN IF NOT EXISTS content_schema_version INTEGER`},
	{"create channel webhook nonce ledger", `CREATE TABLE IF NOT EXISTS channel_webhook_nonces (
		integration_id VARCHAR(255) NOT NULL,
		nonce VARCHAR(128) NOT NULL,
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		PRIMARY KEY (integration_id, nonce)
	)`},
	{"index channel webhook nonce expiry", `CREATE INDEX IF NOT EXISTS idx_channel_webhook_nonces_expiry ON channel_webhook_nonces(expires_at)`},
}

func (m *V54Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v54: workspace database is required")
	}
	workspaceID := "unknown"
	if workspace != nil {
		workspaceID = workspace.ID
	}
	for _, statement := range v54WorkspaceStatements {
		if _, err := db.ExecContext(ctx, statement.query); err != nil {
			return fmt.Errorf("v54: %s for workspace %s: %w", statement.name, workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V54Migration{}) }
