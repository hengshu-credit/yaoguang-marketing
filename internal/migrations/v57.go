package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type V57Migration struct{}

func (m *V57Migration) GetMajorVersion() float64                                       { return 57.0 }
func (m *V57Migration) HasSystemUpdate() bool                                          { return false }
func (m *V57Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V57Migration) ShouldRestartServer() bool                                      { return false }
func (m *V57Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V57Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v57: workspace database is required")
	}
	for _, statement := range schema.ConfigurableMarketingTableDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v57: install configurable marketing: %w", err)
		}
	}
	if workspace != nil && len(workspace.Settings.CustomFieldLabels) > 0 {
		if err := schema.MigrateBusinessAttributeLabels(ctx, db, workspace.Settings.CustomFieldLabels); err != nil {
			return fmt.Errorf("v57: migrate customer attribute labels: %w", err)
		}
	}
	return nil
}

func init() { Register(&V57Migration{}) }
