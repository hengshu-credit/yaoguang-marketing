package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V49Migration adds versioned audiences and campaigns, three independent
// frequency-control scopes, and loss-accounted import staging.
type V49Migration struct{}

func (m *V49Migration) GetMajorVersion() float64                                       { return 49.0 }
func (m *V49Migration) HasSystemUpdate() bool                                          { return false }
func (m *V49Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V49Migration) ShouldRestartServer() bool                                      { return false }
func (m *V49Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

const v49MigrateBroadcastAudiencesSQL = `INSERT INTO audiences (
		id, name, description, kind, status, source_type, source_id, created_at, updated_at
	)
	SELECT gen_random_uuid(), broadcast.name || ' 客群', '由历史营销活动兼容生成',
		'dynamic', 'active', 'broadcast', broadcast.id, broadcast.created_at, broadcast.updated_at
	FROM broadcasts broadcast
	ON CONFLICT (source_type, source_id) WHERE source_type IS NOT NULL AND source_id IS NOT NULL DO NOTHING`

const v49MigrateBroadcastAudienceVersionsSQL = `INSERT INTO audience_versions (
		audience_id, version, definition, definition_hash, created_at
	)
	SELECT audience.id, 1,
		jsonb_build_object('leaf_type', 'list', 'ref_id', COALESCE(broadcast.audience->>'list_id', broadcast.id)),
		encode(sha256(convert_to(jsonb_build_object('leaf_type', 'list', 'ref_id', COALESCE(broadcast.audience->>'list_id', broadcast.id))::text, 'UTF8')), 'hex'),
		broadcast.created_at
	FROM audiences audience
	JOIN broadcasts broadcast ON audience.source_type = 'broadcast' AND audience.source_id = broadcast.id
	ON CONFLICT (audience_id, version) DO NOTHING`

const v49ActivateBroadcastAudiencesSQL = `UPDATE audiences SET active_version = 1, updated_at = CURRENT_TIMESTAMP
	WHERE source_type = 'broadcast' AND active_version = 0`

func (m *V49Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}
	for _, statement := range schema.MarketingTableDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v49: create marketing schema for workspace %s: %w", workspaceID, err)
		}
	}
	for _, migration := range []struct{ label, sql string }{
		{"broadcast audiences", v49MigrateBroadcastAudiencesSQL},
		{"broadcast audience versions", v49MigrateBroadcastAudienceVersionsSQL},
		{"broadcast audience activation", v49ActivateBroadcastAudiencesSQL},
	} {
		if _, err := db.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("v49: migrate %s for workspace %s: %w", migration.label, workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V49Migration{}) }
