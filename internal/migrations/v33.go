package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V33Migration adds inbound reply detection support (stop-on-reply).
//
// Workspace changes (all additive / idempotent):
//   - message_history.smtp_message_id: the recipient-visible RFC Message-ID for
//     reply matching, with a partial index (populated only for exit_on_reply sends).
//   - automations.exit_on_reply: opt-in flag to stop a journey when the contact replies.
//   - a partial unique index on inbound_webhook_events to dedup replayed replies.
//   - redefines track_inbound_webhook_event_changes() so inbound events of type
//     "reply" / "auto_reply" surface on the contact timeline as the dedicated kinds
//     "email.replied" / "email.auto_reply" (other inbound events keep their kind).
//
// The SQL here is kept identical to the fresh-install definitions in
// internal/database/init.go to avoid drift between new and migrated installs.
type V33Migration struct{}

func (m *V33Migration) GetMajorVersion() float64 {
	return 33.0
}

func (m *V33Migration) HasSystemUpdate() bool {
	return false
}

func (m *V33Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V33Migration) ShouldRestartServer() bool {
	return false
}

func (m *V33Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V33Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	statements := []string{
		// Additive columns (nullable / constant default → instant, no table rewrite).
		`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS smtp_message_id VARCHAR(255)`,
		`ALTER TABLE automations ADD COLUMN IF NOT EXISTS exit_on_reply BOOLEAN NOT NULL DEFAULT false`,

		// Partial index for reply matching (near-empty until the feature is used).
		`CREATE INDEX IF NOT EXISTS idx_message_history_smtp_message_id ON message_history(smtp_message_id) WHERE smtp_message_id IS NOT NULL`,

		// Dedup replayed inbound replies/auto-replies.
		`CREATE UNIQUE INDEX IF NOT EXISTS inbound_webhook_events_reply_dedup_idx ON inbound_webhook_events (integration_id, message_id) WHERE type IN ('reply', 'auto_reply') AND message_id IS NOT NULL`,

		// Redefine the trigger function to emit email.replied / email.auto_reply.
		`CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			entity_id_value VARCHAR(255);
			kind_value VARCHAR(50);
		BEGIN
			-- Use message_id if available, otherwise use inbound webhook event id
			entity_id_value := COALESCE(NEW.message_id, NEW.id::text);

			-- Reply / auto-reply events get dedicated timeline kinds so automations
			-- can react to them (stop-on-reply); other inbound events keep the generic kind.
			IF NEW.type = 'reply' THEN
				kind_value := 'email.replied';
			ELSIF NEW.type = 'auto_reply' THEN
				kind_value := 'email.auto_reply';
			ELSE
				kind_value := 'insert_inbound_webhook_event';
			END IF;

			changes_json := jsonb_build_object('type', jsonb_build_object('new', NEW.type), 'source', jsonb_build_object('new', NEW.source));
			IF NEW.bounce_type IS NOT NULL AND NEW.bounce_type != '' THEN changes_json := changes_json || jsonb_build_object('bounce_type', jsonb_build_object('new', NEW.bounce_type)); END IF;
			IF NEW.bounce_category IS NOT NULL AND NEW.bounce_category != '' THEN changes_json := changes_json || jsonb_build_object('bounce_category', jsonb_build_object('new', NEW.bounce_category)); END IF;
			IF NEW.bounce_diagnostic IS NOT NULL AND NEW.bounce_diagnostic != '' THEN changes_json := changes_json || jsonb_build_object('bounce_diagnostic', jsonb_build_object('new', NEW.bounce_diagnostic)); END IF;
			IF NEW.complaint_feedback_type IS NOT NULL AND NEW.complaint_feedback_type != '' THEN changes_json := changes_json || jsonb_build_object('complaint_feedback_type', jsonb_build_object('new', NEW.complaint_feedback_type)); END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.recipient_email, 'insert', 'inbound_webhook_event', kind_value, entity_id_value, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v33 workspace migration failed: %w", err)
		}
	}
	return nil
}

func init() {
	Register(&V33Migration{})
}
