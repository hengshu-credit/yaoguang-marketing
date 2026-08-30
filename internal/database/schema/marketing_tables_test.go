package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarketingTablesCoverAudienceCampaignFrequencyAndImport(t *testing.T) {
	ddl := strings.Join(MarketingTableDefinitions(), ";\n")
	for _, table := range []string{"audiences", "audience_versions", "audience_builds", "audience_memberships", "campaigns", "campaign_versions", "campaign_runs", "campaign_recipient_snapshots", "frequency_policies", "frequency_decisions", "import_jobs", "import_job_rows", "import_job_checkpoints"} {
		assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS "+table)
	}
	assert.Contains(t, ddl, "UNIQUE (run_id, customer_id, variant)")
	assert.Contains(t, ddl, "total_count = pending_count + processing_count + succeeded_count + failed_count")
	assert.Contains(t, ddl, "scope IN ('campaign', 'trigger', 'workspace_global')")
}

func TestMarketingWorkspaceLocalKeysDoNotPretendWorkspaceColumnExists(t *testing.T) {
	ddl := strings.Join(MarketingTableDefinitions(), ";\n")
	assert.NotContains(t, ddl, "workspace_id")
	assert.Contains(t, ddl, "PRIMARY KEY (audience_id, version)")
	assert.Contains(t, ddl, "PRIMARY KEY (job_id, ordinal)")
}
