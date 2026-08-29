package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V45Migration adds encrypted SMS destinations and the idempotent channel-send ledger.
type V45Migration struct{}

func (m *V45Migration) GetMajorVersion() float64                                       { return 45.0 }
func (m *V45Migration) HasSystemUpdate() bool                                          { return false }
func (m *V45Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V45Migration) ShouldRestartServer() bool                                      { return false }
func (m *V45Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

func (m *V45Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}
	statements := []string{
		`SET LOCAL lock_timeout = '5s'`,
		`ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_channel_check`,
		`ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_provider_check`,
		`ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_platform_check`,
		`ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_check`,
		`ALTER TABLE contact_endpoints DROP CONSTRAINT IF EXISTS contact_endpoints_provider_platform_check`,
		`ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_channel_check CHECK (channel IN ('sms', 'push'))`,
		`ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_provider_check CHECK (provider IN ('twilio', 'fcm', 'apns', 'webpush'))`,
		`ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_platform_check CHECK (platform IN ('phone', 'android', 'ios', 'web'))`,
		`ALTER TABLE contact_endpoints ADD CONSTRAINT contact_endpoints_provider_platform_check CHECK (
			(channel = 'sms' AND provider = 'twilio' AND platform = 'phone')
			OR (channel = 'push' AND provider = 'apns' AND platform = 'ios')
			OR (channel = 'push' AND provider = 'fcm' AND platform IN ('android', 'ios'))
			OR (channel = 'push' AND provider = 'webpush' AND platform = 'web'))`,
	}
	statements = append(statements, schema.ChannelSendTableDefinitions()...)
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v45: failed to update workspace %s: %w", workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V45Migration{}) }
