package migrations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V40Migration installs the workspace-local durable event ledger, outbox,
// consumer idempotency, indexed trigger bindings, journey leases, shadow audit,
// and side-effect reservation tables. PostgreSQL remains the online authority;
// every external projection can be rebuilt from these rows.
type V40Migration struct{}

func (m *V40Migration) GetMajorVersion() float64 { return 40.0 }

func (m *V40Migration) HasSystemUpdate() bool { return false }

func (m *V40Migration) HasWorkspaceUpdate() bool { return true }

func (m *V40Migration) ShouldRestartServer() bool { return false }

func (m *V40Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error {
	return nil
}

func (m *V40Migration) UpdateWorkspace(
	ctx context.Context,
	cfg *config.Config,
	workspace *domain.Workspace,
	db DBExecutor,
) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}

	if _, err := db.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("v40: failed to set lock timeout for workspace %s: %w", workspaceID, err)
	}

	if _, err := db.ExecContext(ctx, m.workspaceSQL(workspaceID)); err != nil {
		return fmt.Errorf("v40: failed to install realtime schema for workspace %s: %w", workspaceID, err)
	}

	// Adding a column to contact_timeline changes its composite row type. Rebuild
	// every installed live automation trigger from the authoritative stored
	// configuration so no PL/pgSQL function keeps a stale row descriptor. The v38
	// healer already provides savepoint isolation and refuses unsafe widening;
	// using the current generator also upgrades calls to carry origin_event_id.
	if err := (&V38Migration{}).healAutomationTriggerConditions(ctx, db); err != nil {
		return fmt.Errorf("v40: failed to regenerate automation triggers for workspace %s: %w", workspaceID, err)
	}

	return nil
}

func (m *V40Migration) workspaceSQL(_ string) string {
	statements := append([]string(nil), schema.RealtimeTableDefinitions()...)
	for _, month := range schema.RealtimeBootstrapMonths(time.Now().UTC()) {
		statements = append(statements, schema.EventLedgerPartitionDDL(month))
	}
	return strings.Join(statements, ";\n")
}

func init() {
	Register(&V40Migration{})
}
