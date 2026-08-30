package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomerAuthorityTableDefinitionsAddCustomerReferencesToLegacyMarketingTables(t *testing.T) {
	ddl := strings.Join(CustomerAuthorityTableDefinitions(), ";\n")

	for _, table := range []string{
		"contact_lists",
		"contact_segments",
		"custom_events",
		"contact_timeline",
		"contact_automations",
		"automation_trigger_log",
		"message_history",
		"email_queue",
	} {
		assert.Contains(t, ddl, "ALTER TABLE "+table+" ADD COLUMN IF NOT EXISTS customer_id UUID")
		assert.Contains(t, ddl, "ALTER TABLE "+table+" ADD CONSTRAINT "+table+"_customer_id_fkey")
	}

	assert.NotContains(t, strings.ToUpper(ddl), "DROP COLUMN")
	assert.NotContains(t, strings.ToUpper(ddl), "ALTER COLUMN CUSTOMER_ID SET NOT NULL")
}

func TestCustomerAuthorityTableDefinitionsCreateIndexedReconciliationState(t *testing.T) {
	ddl := strings.Join(CustomerAuthorityTableDefinitions(), ";\n")

	assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS customer_projection_reconciliation")
	assert.Contains(t, ddl, "entity_name VARCHAR(64) PRIMARY KEY")
	assert.Contains(t, ddl, "missing_customer_id_count BIGINT NOT NULL DEFAULT 0")
	assert.Contains(t, ddl, "conflict_count BIGINT NOT NULL DEFAULT 0")
	assert.Contains(t, ddl, "last_scanned_at TIMESTAMPTZ")
	assert.Contains(t, ddl, "last_error TEXT")
	assert.Contains(t, ddl, "CREATE INDEX IF NOT EXISTS idx_customer_projection_reconciliation_attention")
}

func TestCustomerAuthorityTableDefinitionsPopulateTimelineCustomerReference(t *testing.T) {
	ddl := strings.Join(CustomerAuthorityTableDefinitions(), ";\n")

	assert.Contains(t, ddl, "CREATE OR REPLACE FUNCTION populate_contact_timeline_customer_id()")
	assert.Contains(t, ddl, "SELECT customer_id INTO NEW.customer_id FROM contacts WHERE email = NEW.email")
	assert.Contains(t, ddl, "BEFORE INSERT OR UPDATE OF email ON contact_timeline")
}
