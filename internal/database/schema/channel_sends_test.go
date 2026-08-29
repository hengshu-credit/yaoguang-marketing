package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelSendTableDefinitions(t *testing.T) {
	sql := strings.Join(ChannelSendTableDefinitions(), ";\n")
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS channel_send_executions")
	assert.Contains(t, sql, "effect_key VARCHAR(255) PRIMARY KEY")
	assert.Contains(t, sql, "request_hash CHAR(64) NOT NULL")
	assert.Contains(t, sql, "idx_channel_send_executions_message")
}
