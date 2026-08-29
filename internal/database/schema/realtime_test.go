package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRealtimeTableDefinitionsContainAuthorityAndIdempotencyConstraints(t *testing.T) {
	ddl := strings.Join(RealtimeTableDefinitions(), "\n")

	assert.Contains(t, ddl, "event_idempotency")
	assert.Contains(t, ddl, "id UUID PRIMARY KEY")
	assert.Contains(t, ddl, "UNIQUE (event_id, topic, routing_key)")
	assert.Contains(t, ddl, "PRIMARY KEY (consumer, message_id)")
	assert.Contains(t, ddl, "PRIMARY KEY (automation_id, automation_version, event_type, subject_type)")
	assert.Contains(t, ddl, "UNIQUE (event_id, automation_id, engine)")
	assert.Contains(t, ddl, "effect_key TEXT PRIMARY KEY")
}

func TestTimelineEventBridgeIsFixedCostAndWritesLedgerAndOutbox(t *testing.T) {
	ddl := TimelineEventBridgeFunction()

	assert.Contains(t, ddl, "INSERT INTO event_idempotency")
	assert.Contains(t, ddl, "INSERT INTO event_ledger")
	assert.Contains(t, ddl, "INSERT INTO event_outbox")
	assert.NotContains(t, strings.ToLower(ddl), "from automations")
	assert.NotContains(t, strings.ToLower(ddl), "automation_trigger_bindings")
}
