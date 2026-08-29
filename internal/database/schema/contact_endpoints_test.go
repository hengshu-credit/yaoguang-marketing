package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContactEndpointTableDefinitions(t *testing.T) {
	sql := strings.Join(ContactEndpointTableDefinitions(), ";\n")
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS contact_endpoints")
	assert.Contains(t, sql, "address_ciphertext TEXT NOT NULL")
	assert.Contains(t, sql, "address_fingerprint CHAR(64) NOT NULL")
	assert.Contains(t, sql, "WHERE enabled")
	assert.Contains(t, sql, "contact.endpoint_registered")
	assert.Contains(t, sql, "contact.endpoint_updated")
	assert.Contains(t, sql, "contact.endpoint_disabled")
	assert.NotContains(t, sql, "address_ciphertext',")
	assert.NotContains(t, sql, "address_fingerprint',")
}
