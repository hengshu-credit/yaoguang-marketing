package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAudienceIngestTableDefinitions(t *testing.T) {
	sql := strings.Join(AudienceIngestTableDefinitions(), ";\n")
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS contact_profiles")
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS contact_tags")
	assert.Contains(t, sql, "USING GIN (attributes jsonb_path_ops)")
	assert.Contains(t, sql, "contact.profile_created")
	assert.Contains(t, sql, "contact.profile_updated")
	assert.Contains(t, sql, "contact.tagged")
	assert.Contains(t, sql, "contact.untagged")
	assert.Contains(t, sql, "INSERT INTO contact_timeline")
}
