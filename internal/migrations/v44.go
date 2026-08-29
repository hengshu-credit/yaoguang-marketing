package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V44Migration adds native SMS/push delivery receipt persistence and lookup indexes.
type V44Migration struct{}

func (m *V44Migration) GetMajorVersion() float64                                       { return 44.0 }
func (m *V44Migration) HasSystemUpdate() bool                                          { return false }
func (m *V44Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V44Migration) ShouldRestartServer() bool                                      { return false }
func (m *V44Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V44Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}
	statements := []string{
		`SET LOCAL lock_timeout = '5s'`,
		`CREATE TABLE IF NOT EXISTS delivery_receipts (
			provider VARCHAR(32) NOT NULL,
			receipt_id VARCHAR(255) NOT NULL,
			provider_message_id VARCHAR(255),
			message_id VARCHAR(255),
			effect_key VARCHAR(255),
			event VARCHAR(32) NOT NULL,
			occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
			received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			error_code VARCHAR(255),
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			payload_hash CHAR(64) NOT NULL,
			PRIMARY KEY (provider, receipt_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_receipts_provider_message
			ON delivery_receipts(provider, provider_message_id)
			WHERE provider_message_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_receipts_message
			ON delivery_receipts(message_id, occurred_at DESC)
			WHERE message_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_external_id
			ON message_history(external_id)
			WHERE external_id IS NOT NULL`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v44: failed to update workspace %s: %w", workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V44Migration{}) }
