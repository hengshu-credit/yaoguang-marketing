package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type V56Migration struct{}

func (m *V56Migration) GetMajorVersion() float64                                       { return 56.0 }
func (m *V56Migration) HasSystemUpdate() bool                                          { return false }
func (m *V56Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V56Migration) ShouldRestartServer() bool                                      { return false }
func (m *V56Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V56Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, _ *domain.Workspace, db DBExecutor) error {
	if db == nil {
		return fmt.Errorf("v56: workspace database is required")
	}
	for _, statement := range schema.CustomerActivityTimelineDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v56: install customer activity triggers: %w", err)
		}
	}
	return nil
}

func init() { Register(&V56Migration{}) }
