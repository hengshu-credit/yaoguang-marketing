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

func TestMarketingTablesSupportListCampaignSourcesAndImportBindings(t *testing.T) {
	ddl := strings.Join(MarketingTableDefinitions(), ";\n")
	assert.Contains(t, ddl, "list_id VARCHAR(32)")
	assert.Contains(t, ddl, "list_ids TEXT[] NOT NULL DEFAULT '{}'::text[]")
	assert.Contains(t, ddl, "CONSTRAINT campaign_versions_source_check CHECK")
	assert.Contains(t, ddl, "audience_id IS NOT NULL AND audience_version IS NOT NULL AND list_id IS NULL")
	assert.Contains(t, ddl, "audience_id IS NULL AND audience_version IS NULL AND list_id IS NOT NULL")
}

func TestMarketingTablesPersistResolvedAudienceSourceOnCampaignRuns(t *testing.T) {
	ddl := strings.Join(MarketingTableDefinitions(), ";\n")
	assert.Contains(t, ddl, "audience_id UUID")
	assert.Contains(t, ddl, "audience_version INTEGER")
	assert.Contains(t, ddl, "audience_build_id UUID")
	assert.Contains(t, ddl, "idx_campaign_runs_audience_build")
}
