package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomerTableDefinitionsCreateAggregateIdentityAndIdempotencySchema(t *testing.T) {
	sql := strings.Join(CustomerTableDefinitions(), ";\n")

	for _, table := range []string{
		"customers",
		"customer_profiles",
		"customer_identities",
		"customer_tags",
		"customer_consents",
		"customer_list_memberships",
		"customer_merge_log",
		"customer_idempotency",
	} {
		assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS "+table)
	}

	assert.Contains(t, sql, "id UUID PRIMARY KEY")
	assert.Contains(t, sql, "customer_no VARCHAR(53) NOT NULL")
	assert.Contains(t, sql, "external_user_id VARCHAR(255)")
	assert.Contains(t, sql, "merged_into_id UUID")
	assert.Contains(t, sql, "version BIGINT NOT NULL DEFAULT 1")
	assert.Contains(t, sql, "attributes JSONB NOT NULL DEFAULT '{}'::jsonb")
	assert.Contains(t, sql, "identity_type VARCHAR(32) NOT NULL")
	assert.Contains(t, sql, "value_ciphertext TEXT NOT NULL")
	assert.Contains(t, sql, "lookup_fingerprint CHAR(64) NOT NULL")
	assert.Contains(t, sql, "display_hint VARCHAR(255) NOT NULL")
	assert.Contains(t, sql, "payload_hash CHAR(64) NOT NULL")
	assert.Contains(t, sql, "PRIMARY KEY (operation, idempotency_key)")
}

func TestCustomerTableDefinitionsEnforceWorkspaceLocalUniquenessAndRelationships(t *testing.T) {
	sql := strings.Join(CustomerTableDefinitions(), ";\n")

	assert.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_customers_customer_no")
	assert.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_customers_external_user_id")
	assert.Contains(t, sql, "WHERE external_user_id IS NOT NULL")
	assert.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_identities_lookup")
	assert.Contains(t, sql, "ON customer_identities (identity_type, lookup_fingerprint)")
	assert.Contains(t, sql, "REFERENCES customers(id) ON DELETE CASCADE")
	assert.Contains(t, sql, "REFERENCES lists(id)")
	assert.Contains(t, sql, "REFERENCES customers(id) ON DELETE RESTRICT")
}

func TestCustomerTableDefinitionsAddUUIDCompatibilityProjectionWithoutRemovingEmailKeys(t *testing.T) {
	sql := strings.Join(CustomerTableDefinitions(), ";\n")

	assert.Contains(t, sql, "ALTER TABLE contacts ADD COLUMN IF NOT EXISTS customer_id UUID")
	assert.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS uq_contacts_customer_id")
	assert.Contains(t, sql, "ALTER TABLE contact_endpoints ADD COLUMN IF NOT EXISTS customer_id UUID")
	assert.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_contact_endpoints_customer_id")
	assert.Contains(t, sql, "ADD CONSTRAINT contacts_customer_id_fkey")
	assert.Contains(t, sql, "ADD CONSTRAINT contact_endpoints_customer_id_fkey")
	assert.Contains(t, sql, "FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL")
	assert.NotContains(t, strings.ToUpper(sql), "DROP COLUMN EMAIL")
	assert.NotContains(t, strings.ToUpper(sql), "DROP TABLE CONTACTS")
}
