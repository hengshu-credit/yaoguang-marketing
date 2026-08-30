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
}

func TestJourneyEnrollFunctionResolvesCustomerAndKeepsLegacySignature(t *testing.T) {
	function := JourneyAutomationEnrollContactFunction()
	assert.Contains(t, function, "p_contact_email VARCHAR(255)")
	assert.Contains(t, function, "p_origin_event_id UUID DEFAULT NULL")
	assert.Contains(t, function, "SELECT customer_id INTO v_customer_id FROM contacts")
	assert.Contains(t, function, "journey_enrollments")
	assert.Contains(t, function, "ON CONFLICT DO NOTHING")
}
