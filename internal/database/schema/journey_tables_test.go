package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJourneySchemaUsesDatabaseUniqueConstraintsForBothFrequencies(t *testing.T) {
	ddl := strings.Join(JourneyTableDefinitions(), "\n")
	assert.Contains(t, ddl, "journey_enrollments")
	assert.Contains(t, ddl, "automation_id, customer_id")
	assert.Contains(t, ddl, "WHERE frequency = 'once'")
	assert.Contains(t, ddl, "automation_id, customer_id, origin_event_id")
	assert.Contains(t, ddl, "WHERE frequency = 'every_time'")
	assert.Contains(t, ddl, "ADD COLUMN IF NOT EXISTS customer_id UUID")
	assert.Contains(t, ddl, "ALTER TABLE event_ledger ADD COLUMN IF NOT EXISTS customer_id UUID")
	assert.Contains(t, ddl, "contact_automation_journey_projection")
	assert.Contains(t, ddl, "INSERT INTO journey_instance_events")
}

func TestJourneyEnrollFunctionResolvesCustomerAndKeepsLegacySignature(t *testing.T) {
	function := JourneyAutomationEnrollContactFunction()
	assert.Contains(t, function, "CREATE OR REPLACE FUNCTION automation_enroll_customer(")
	assert.Contains(t, function, "p_customer_id UUID")
	assert.Contains(t, function, "RETURNS TABLE(outcome TEXT, contact_automation_id VARCHAR(36), retry_at TIMESTAMPTZ)")
	assert.Contains(t, function, "p_contact_email VARCHAR(255)")
	assert.Contains(t, function, "p_origin_event_id UUID DEFAULT NULL")
	assert.Contains(t, function, "SELECT customer_id INTO v_customer_id FROM contacts")
	assert.Contains(t, function, "journey_enrollments")
	assert.Contains(t, function, "ON CONFLICT DO NOTHING")
}
