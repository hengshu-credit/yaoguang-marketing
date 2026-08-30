package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V47Migration adds Customer UUID references to legacy marketing tables while
// keeping their email keys available during the compatibility period.
type V47Migration struct{}

func (m *V47Migration) GetMajorVersion() float64                                       { return 47.0 }
func (m *V47Migration) HasSystemUpdate() bool                                          { return false }
func (m *V47Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V47Migration) ShouldRestartServer() bool                                      { return false }
func (m *V47Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

type v47CustomerReference struct {
	table       string
	emailColumn string
}

var v47CustomerReferences = []v47CustomerReference{
	{table: "contact_lists", emailColumn: "email"},
	{table: "contact_segments", emailColumn: "email"},
	{table: "custom_events", emailColumn: "email"},
	{table: "contact_timeline", emailColumn: "email"},
	{table: "contact_automations", emailColumn: "contact_email"},
	{table: "automation_trigger_log", emailColumn: "contact_email"},
	{table: "message_history", emailColumn: "contact_email"},
	{table: "email_queue", emailColumn: "contact_email"},
}

func (m *V47Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}

	for _, statement := range schema.CustomerAuthorityTableDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v47: create customer authority compatibility schema for workspace %s: %w", workspaceID, err)
		}
	}

	for _, reference := range v47CustomerReferences {
		backfill := fmt.Sprintf("UPDATE %s legacy SET customer_id = c.customer_id FROM contacts c WHERE legacy.customer_id IS NULL AND c.customer_id IS NOT NULL AND LOWER(BTRIM(legacy.%s)) = LOWER(BTRIM(c.email))", reference.table, reference.emailColumn)
		if _, err := db.ExecContext(ctx, backfill); err != nil {
			return fmt.Errorf("v47: backfill %s customer references for workspace %s: %w", reference.table, workspaceID, err)
		}

		reconcile := fmt.Sprintf("INSERT INTO customer_projection_reconciliation (entity_name, missing_customer_id_count, conflict_count, last_scanned_at, last_error, updated_at) SELECT $1, COUNT(*) FILTER (WHERE legacy.customer_id IS NULL), COUNT(*) FILTER (WHERE legacy.customer_id IS NOT NULL AND c.customer_id IS NOT NULL AND legacy.customer_id <> c.customer_id), CURRENT_TIMESTAMP, NULL, CURRENT_TIMESTAMP FROM %s legacy LEFT JOIN contacts c ON LOWER(BTRIM(legacy.%s)) = LOWER(BTRIM(c.email)) ON CONFLICT (entity_name) DO UPDATE SET missing_customer_id_count = EXCLUDED.missing_customer_id_count, conflict_count = EXCLUDED.conflict_count, last_scanned_at = EXCLUDED.last_scanned_at, last_error = NULL, updated_at = EXCLUDED.updated_at", reference.table, reference.emailColumn)
		if _, err := db.ExecContext(ctx, reconcile, reference.table); err != nil {
			return fmt.Errorf("v47: reconcile %s customer references for workspace %s: %w", reference.table, workspaceID, err)
		}
	}

	return nil
}

func init() { Register(&V47Migration{}) }
