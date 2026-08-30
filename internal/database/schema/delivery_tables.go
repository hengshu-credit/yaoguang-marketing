package schema

// DeliveryTableDefinitions is shared by fresh workspace creation and v48
// upgrades. Delivery intents are the logical exactly-once boundary; attempts
// are provider submissions and reconciliations represent uncertain outcomes.
func DeliveryTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS delivery_intents (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			effect_key CHAR(64) NOT NULL UNIQUE,
			request_hash CHAR(64) NOT NULL,
			source_type VARCHAR(32) NOT NULL,
			source_id VARCHAR(255) NOT NULL,
			source_version VARCHAR(128) NOT NULL,
			customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
			legacy_identity VARCHAR(255),
			channel VARCHAR(20) NOT NULL CHECK (channel IN (
				'email', 'sms', 'push', 'whatsapp', 'telegram', 'in_app', 'webhook'
			)),
			template_id VARCHAR(255),
			template_version BIGINT CHECK (template_version IS NULL OR template_version > 0),
			node_or_phase VARCHAR(255) NOT NULL,
			occurrence VARCHAR(255) NOT NULL,
			variant VARCHAR(255) NOT NULL,
			status VARCHAR(24) NOT NULL CHECK (status IN (
				'planned', 'reserved', 'queued', 'submitting', 'provider_accepted',
				'confirmed', 'suppressed', 'deferred', 'transient_failed',
				'terminal_failed', 'unknown', 'cancelled'
			)),
			suppression_reason VARCHAR(255),
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_intents_source
			ON delivery_intents(source_type, source_id, source_version, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_intents_status
			ON delivery_intents(status, updated_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_intents_customer
			ON delivery_intents(customer_id, created_at DESC)
			WHERE customer_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS delivery_attempts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			intent_id UUID NOT NULL REFERENCES delivery_intents(id) ON DELETE CASCADE,
			attempt_no INTEGER NOT NULL CHECK (attempt_no > 0),
			provider VARCHAR(64) NOT NULL,
			request_hash CHAR(64) NOT NULL,
			provider_message_id VARCHAR(255),
			status VARCHAR(24) NOT NULL CHECK (status IN (
				'reserved', 'queued', 'submitting', 'provider_accepted', 'confirmed',
				'transient_failed', 'terminal_failed', 'unknown', 'cancelled'
			)),
			claim_token UUID,
			lease_expires_at TIMESTAMPTZ,
			submitted_at TIMESTAMPTZ,
			accepted_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			error_category VARCHAR(64),
			error_code VARCHAR(255),
			error_detail TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(intent_id, attempt_no)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_attempts_provider_message
			ON delivery_attempts(provider, provider_message_id)
			WHERE provider_message_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_attempts_lease
			ON delivery_attempts(status, lease_expires_at)
			WHERE status IN ('reserved', 'queued', 'submitting')`,
		`CREATE TABLE IF NOT EXISTS delivery_reconciliations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			intent_id UUID NOT NULL REFERENCES delivery_intents(id) ON DELETE CASCADE,
			attempt_id UUID REFERENCES delivery_attempts(id) ON DELETE SET NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN (
				'pending', 'querying', 'resolved', 'manual'
			)),
			resolution VARCHAR(64),
			actor_id VARCHAR(255),
			reason TEXT,
			provider_result JSONB NOT NULL DEFAULT '{}'::jsonb,
			next_query_at TIMESTAMPTZ,
			lease_token UUID,
			lease_expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_delivery_reconciliations_pending
			ON delivery_reconciliations(status, next_query_at, lease_expires_at, created_at)
			WHERE status IN ('pending', 'querying', 'manual')`,
		`ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS delivery_intent_id UUID`,
		`DO $$ BEGIN
			ALTER TABLE email_queue ADD CONSTRAINT fk_email_queue_delivery_intent
				FOREIGN KEY (delivery_intent_id) REFERENCES delivery_intents(id) ON DELETE RESTRICT;
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$`,
		`ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS claim_token UUID`,
		`ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ`,
		`ALTER TABLE email_queue ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_email_queue_delivery_intent
			ON email_queue(delivery_intent_id) WHERE delivery_intent_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_email_queue_claim
			ON email_queue(status, lease_expires_at, priority, created_at)
			WHERE status IN ('pending', 'failed', 'processing')`,
	}
}
