package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type V50Migration struct{}

func (m *V50Migration) GetMajorVersion() float64                                       { return 50.0 }
func (m *V50Migration) HasSystemUpdate() bool                                          { return false }
func (m *V50Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V50Migration) ShouldRestartServer() bool                                      { return false }
func (m *V50Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

var v50JourneyBackfillStatements = []string{
	`UPDATE contact_automations journey SET customer_id = contact.customer_id
		FROM contacts contact WHERE journey.customer_id IS NULL AND contact.email = journey.contact_email AND contact.customer_id IS NOT NULL`,
	`UPDATE automation_trigger_log log SET customer_id = contact.customer_id
		FROM contacts contact WHERE log.customer_id IS NULL AND contact.email = log.contact_email AND contact.customer_id IS NOT NULL`,
	`INSERT INTO journey_identity_reconciliation (automation_id, contact_email, origin_event_id, reason, first_seen_at, last_seen_at)
		SELECT journey.automation_id, journey.contact_email,
			(array_agg(journey.origin_event_id ORDER BY journey.entered_at DESC))[1],
			'historical_customer_not_resolved', MIN(journey.entered_at), MAX(journey.entered_at)
		FROM contact_automations journey WHERE journey.customer_id IS NULL
		GROUP BY journey.automation_id, journey.contact_email
		ON CONFLICT (automation_id, contact_email) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, reason = EXCLUDED.reason`,
	`INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, dedupe_key, entered_at, created_at)
		SELECT gen_random_uuid(), log.automation_id, COALESCE(automation.version, 1), log.customer_id, log.contact_email, 'once',
			encode(sha256(convert_to(concat_ws(':', log.automation_id, log.customer_id::text, 'once'), 'UTF8')), 'hex'), log.triggered_at, log.triggered_at
		FROM automation_trigger_log log JOIN automations automation ON automation.id = log.automation_id
		WHERE log.customer_id IS NOT NULL ON CONFLICT DO NOTHING`,
	`INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, origin_event_id, dedupe_key, entered_at, created_at)
		SELECT gen_random_uuid(), journey.automation_id, journey.automation_version, journey.customer_id, journey.contact_email, 'every_time', journey.origin_event_id,
			encode(sha256(convert_to(concat_ws(':', journey.automation_id, journey.customer_id::text, 'every_time', journey.origin_event_id::text), 'UTF8')), 'hex'), journey.entered_at, journey.entered_at
		FROM contact_automations journey WHERE journey.customer_id IS NOT NULL AND journey.origin_event_id IS NOT NULL ON CONFLICT DO NOTHING`,
	`INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, dedupe_key, entered_at, created_at)
		SELECT gen_random_uuid(), journey.automation_id, journey.automation_version, journey.customer_id, journey.contact_email, 'once',
			encode(sha256(convert_to(concat_ws(':', journey.automation_id, journey.customer_id::text, 'once'), 'UTF8')), 'hex'), journey.entered_at, journey.entered_at
		FROM contact_automations journey WHERE journey.customer_id IS NOT NULL AND journey.origin_event_id IS NULL ON CONFLICT DO NOTHING`,
	`INSERT INTO journey_instances (id, enrollment_id, contact_automation_id, status, current_node_id, next_scheduled_at, started_at, completed_at, created_at, updated_at)
		SELECT gen_random_uuid(), enrollment.id, journey.id, journey.status, journey.current_node_id, journey.scheduled_at, journey.entered_at,
			CASE WHEN journey.status IN ('completed', 'exited', 'failed') THEN COALESCE(journey.scheduled_at, journey.entered_at) END,
			journey.entered_at, CURRENT_TIMESTAMP
		FROM contact_automations journey JOIN journey_enrollments enrollment ON enrollment.automation_id = journey.automation_id
			AND enrollment.customer_id = journey.customer_id
			AND ((enrollment.frequency = 'every_time' AND enrollment.origin_event_id = journey.origin_event_id)
				OR (enrollment.frequency = 'once' AND journey.origin_event_id IS NULL))
		WHERE journey.customer_id IS NOT NULL ON CONFLICT DO NOTHING`,
}

func (m *V50Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}
	for _, statement := range schema.JourneyTableDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v50: create journey schema for workspace %s: %w", workspaceID, err)
		}
	}
	for _, statement := range v50JourneyBackfillStatements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v50: backfill journey authority for workspace %s: %w", workspaceID, err)
		}
	}
	if _, err := db.ExecContext(ctx, schema.JourneyAutomationEnrollContactFunction()); err != nil {
		return fmt.Errorf("v50: install customer journey enrollment for workspace %s: %w", workspaceID, err)
	}
	return nil
}

func init() { Register(&V50Migration{}) }
