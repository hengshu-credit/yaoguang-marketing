package schema

// ChannelSendTableDefinitions is shared by fresh workspace creation and v45 upgrades.
func ChannelSendTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS channel_send_executions (
			effect_key VARCHAR(255) PRIMARY KEY,
			request_hash CHAR(64) NOT NULL,
			message_id VARCHAR(255) NOT NULL UNIQUE,
			channel VARCHAR(20) NOT NULL CHECK (channel IN ('sms', 'push')),
			integration_id VARCHAR(255) NOT NULL,
			contact_email VARCHAR(255) NOT NULL REFERENCES contacts(email) ON DELETE CASCADE,
			endpoint_id VARCHAR(128) NOT NULL REFERENCES contact_endpoints(endpoint_id),
			template_id VARCHAR(255) NOT NULL,
			template_version BIGINT NOT NULL CHECK (template_version > 0),
			language VARCHAR(35),
			status VARCHAR(20) NOT NULL CHECK (status IN ('reserved', 'submitted', 'confirmed', 'failed', 'unknown')),
			provider VARCHAR(32),
			provider_message_id VARCHAR(255),
			attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			last_error VARCHAR(1000),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_send_executions_message
			ON channel_send_executions(provider, provider_message_id)
			WHERE provider_message_id IS NOT NULL`,
	}
}
