package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V42Migration adds encrypted client endpoints for omnichannel delivery.
type V42Migration struct{}

func (m *V42Migration) GetMajorVersion() float64                                       { return 42.0 }
func (m *V42Migration) HasSystemUpdate() bool                                          { return false }
func (m *V42Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V42Migration) ShouldRestartServer() bool                                      { return false }
func (m *V42Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V42Migration) UpdateWorkspace(
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
		return fmt.Errorf("v42: failed to set lock timeout for workspace %s: %w", workspaceID, err)
	}
	if _, err := db.ExecContext(ctx, strings.Join(schema.ContactEndpointTableDefinitions(), ";\n")); err != nil {
		return fmt.Errorf("v42: failed to install contact endpoint schema for workspace %s: %w", workspaceID, err)
	}
	return nil
}

func init() { Register(&V42Migration{}) }
