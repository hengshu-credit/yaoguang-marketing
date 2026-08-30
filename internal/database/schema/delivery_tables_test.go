package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeliveryTableDefinitionsCreateUnifiedLedger(t *testing.T) {
	sql := strings.Join(DeliveryTableDefinitions(), ";\n")

	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS delivery_intents")
	assert.Contains(t, sql, "effect_key CHAR(64) NOT NULL UNIQUE")
	assert.Contains(t, sql, "customer_id UUID REFERENCES customers(id)")
	assert.Contains(t, sql, "source_version VARCHAR(128) NOT NULL")
	assert.Contains(t, sql, "occurrence VARCHAR(255) NOT NULL")
	assert.Contains(t, sql, "provider_accepted")
	assert.Contains(t, sql, "terminal_failed")
	assert.Contains(t, sql, "unknown")

	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS delivery_attempts")
	assert.Contains(t, sql, "UNIQUE(intent_id, attempt_no)")
	assert.Contains(t, sql, "provider_message_id VARCHAR(255)")
	assert.Contains(t, sql, "claim_token UUID")
	assert.Contains(t, sql, "lease_expires_at TIMESTAMPTZ")
	assert.Contains(t, sql, "error_category VARCHAR(64)")

	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS delivery_reconciliations")
	assert.Contains(t, sql, "provider_result JSONB")
	assert.Contains(t, sql, "resolution VARCHAR(64)")
	assert.Contains(t, sql, "actor_id VARCHAR(255)")
}

func TestDeliveryTableDefinitionsUpgradeEmailQueueForLeaseClaims(t *testing.T) {
	sql := strings.Join(DeliveryTableDefinitions(), ";\n")

	assert.Contains(t, sql, "ADD COLUMN IF NOT EXISTS delivery_intent_id UUID")
	assert.Contains(t, sql, "ADD COLUMN IF NOT EXISTS claim_token UUID")
	assert.Contains(t, sql, "ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ")
	assert.Contains(t, sql, "ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ")
	assert.Contains(t, sql, "FOREIGN KEY (delivery_intent_id) REFERENCES delivery_intents(id)")
	assert.Contains(t, sql, "idx_email_queue_delivery_intent")
	assert.Contains(t, sql, "idx_email_queue_claim")
	assert.Contains(t, sql, "status IN ('pending', 'failed', 'processing')")
}
