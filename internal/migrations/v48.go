package migrations

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V48Migration introduces a single logical delivery ledger for every channel.
// Existing channel-send and email-queue rows are retained as legacy intents so
// operators do not lose the evidence needed to explain historical sends.
type V48Migration struct{}

func (m *V48Migration) GetMajorVersion() float64                                       { return 48.0 }
func (m *V48Migration) HasSystemUpdate() bool                                          { return false }
func (m *V48Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V48Migration) ShouldRestartServer() bool                                      { return false }
func (m *V48Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

const v48MigrateChannelSendsSQL = `INSERT INTO delivery_intents (
		effect_key, request_hash, source_type, source_id, source_version,
		customer_id, legacy_identity, channel, template_id, template_version,
		node_or_phase, occurrence, variant, status, metadata, created_at, updated_at
	)
	SELECT
		encode(sha256(convert_to('legacy-channel-send:' || execution.effect_key, 'UTF8')), 'hex'),
		execution.request_hash,
		'legacy_channel_send', execution.message_id, 'legacy',
		contact.customer_id,
		CASE WHEN contact.customer_id IS NULL THEN execution.contact_email ELSE NULL END,
		execution.channel, execution.template_id, execution.template_version,
		'channel_send', execution.effect_key, 'default',
		CASE execution.status
			WHEN 'reserved' THEN 'reserved'
			WHEN 'submitted' THEN 'submitting'
			WHEN 'confirmed' THEN 'confirmed'
			WHEN 'failed' THEN 'transient_failed'
			WHEN 'unknown' THEN 'unknown'
			ELSE 'unknown'
		END,
		jsonb_strip_nulls(jsonb_build_object(
			'legacy_effect_key', execution.effect_key,
			'legacy_message_id', execution.message_id,
			'integration_id', execution.integration_id,
			'endpoint_id', execution.endpoint_id,
			'provider', execution.provider,
			'provider_message_id', execution.provider_message_id,
			'legacy_identity', CASE WHEN contact.customer_id IS NULL THEN execution.contact_email ELSE NULL END
		)),
		execution.created_at, execution.updated_at
	FROM channel_send_executions execution
	LEFT JOIN contacts contact ON contact.email = execution.contact_email
	ON CONFLICT (effect_key) DO NOTHING`

const v48MigrateEmailQueueSQL = `INSERT INTO delivery_intents (
		effect_key, request_hash, source_type, source_id, source_version,
		customer_id, legacy_identity, channel, template_id, template_version,
		node_or_phase, occurrence, variant, status, metadata, created_at, updated_at
	)
	SELECT
		encode(sha256(convert_to('legacy-email-queue:' || queue.id, 'UTF8')), 'hex'),
		encode(sha256(convert_to(queue.payload::text, 'UTF8')), 'hex'),
		queue.source_type, queue.source_id, 'legacy',
		queue.customer_id,
		CASE WHEN queue.customer_id IS NULL THEN queue.contact_email ELSE NULL END,
		'email', queue.template_id, NULL,
		'email_queue', queue.id, 'default',
		CASE queue.status
			WHEN 'pending' THEN 'queued'
			WHEN 'processing' THEN 'submitting'
			WHEN 'failed' THEN 'transient_failed'
			WHEN 'paused' THEN 'deferred'
			WHEN 'completed' THEN 'confirmed'
			WHEN 'processed' THEN 'confirmed'
			ELSE 'unknown'
		END,
		jsonb_strip_nulls(jsonb_build_object(
			'legacy_email_queue_id', queue.id,
			'legacy_source_type', queue.source_type,
			'legacy_identity', CASE WHEN queue.customer_id IS NULL THEN queue.contact_email ELSE NULL END
		)),
		queue.created_at, queue.updated_at
	FROM email_queue queue
	ON CONFLICT (effect_key) DO NOTHING`

const v48LinkEmailQueueSQL = `UPDATE email_queue queue
	SET delivery_intent_id = intent.id
	FROM delivery_intents intent
	WHERE queue.delivery_intent_id IS NULL
		AND intent.effect_key = encode(sha256(convert_to('legacy-email-queue:' || queue.id, 'UTF8')), 'hex')`

func (m *V48Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	workspaceID := ""
	if workspace != nil {
		workspaceID = workspace.ID
	}

	for _, statement := range schema.DeliveryTableDefinitions() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v48: create delivery ledger for workspace %s: %w", workspaceID, err)
		}
	}
	migrations := []struct {
		label     string
		statement string
	}{
		{label: "channel send executions", statement: v48MigrateChannelSendsSQL},
		{label: "email queue", statement: v48MigrateEmailQueueSQL},
		{label: "email queue links", statement: v48LinkEmailQueueSQL},
	}
	for _, migration := range migrations {
		if _, err := db.ExecContext(ctx, migration.statement); err != nil {
			return fmt.Errorf("v48: migrate %s for workspace %s: %w", migration.label, workspaceID, err)
		}
	}
	return nil
}

func init() { Register(&V48Migration{}) }
