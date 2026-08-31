package schema

// MarketingTableDefinitions is shared by fresh workspace initialization and
// the V49 migration. The workspace database itself is the isolation boundary.
func MarketingTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS audiences (
			id UUID PRIMARY KEY, name VARCHAR(255) NOT NULL, description TEXT,
			kind VARCHAR(20) NOT NULL CHECK (kind IN ('static', 'dynamic', 'composite')),
			status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
			active_version INTEGER NOT NULL DEFAULT 0 CHECK (active_version >= 0), active_build_id UUID,
			source_type VARCHAR(40), source_id VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_audiences_source ON audiences(source_type, source_id) WHERE source_type IS NOT NULL AND source_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS audience_versions (
			audience_id UUID NOT NULL REFERENCES audiences(id) ON DELETE RESTRICT, version INTEGER NOT NULL CHECK (version > 0),
			definition JSONB NOT NULL, definition_hash CHAR(64) NOT NULL, created_by VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (audience_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS audience_builds (
			id UUID PRIMARY KEY, audience_id UUID NOT NULL, audience_version INTEGER NOT NULL,
			status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'building', 'completed', 'failed', 'cancelled')),
			last_customer_id UUID, member_count BIGINT NOT NULL DEFAULT 0 CHECK (member_count >= 0), error_detail TEXT,
			started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (audience_id, audience_version) REFERENCES audience_versions(audience_id, version) ON DELETE RESTRICT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audience_builds_resume ON audience_builds(status, updated_at) WHERE status IN ('pending', 'building')`,
		`CREATE TABLE IF NOT EXISTS audience_memberships (
			build_id UUID NOT NULL REFERENCES audience_builds(id) ON DELETE CASCADE,
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE, ordinal BIGINT NOT NULL CHECK (ordinal > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (build_id, customer_id), UNIQUE (build_id, ordinal)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audience_memberships_customer ON audience_memberships(customer_id, build_id)`,
		`CREATE TABLE IF NOT EXISTS campaigns (
			id UUID PRIMARY KEY, name VARCHAR(255) NOT NULL,
			status VARCHAR(20) NOT NULL CHECK (status IN ('draft', 'scheduled', 'running', 'paused', 'completed', 'cancelled')),
			draft_version INTEGER NOT NULL DEFAULT 1 CHECK (draft_version > 0), active_version INTEGER CHECK (active_version > 0),
			source_type VARCHAR(40), source_id VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_campaigns_source ON campaigns(source_type, source_id) WHERE source_type IS NOT NULL AND source_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS campaign_versions (
			campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE RESTRICT, version INTEGER NOT NULL CHECK (version > 0),
			audience_id UUID, audience_version INTEGER, list_id VARCHAR(32), channel VARCHAR(20) NOT NULL,
			variants JSONB NOT NULL, config JSONB NOT NULL DEFAULT '{}'::jsonb, activated_at TIMESTAMPTZ,
			created_by VARCHAR(255), created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (campaign_id, version),
			CONSTRAINT campaign_versions_source_check CHECK (
				(audience_id IS NOT NULL AND audience_version IS NOT NULL AND list_id IS NULL)
				OR (audience_id IS NULL AND audience_version IS NULL AND list_id IS NOT NULL)
			),
			FOREIGN KEY (audience_id, audience_version) REFERENCES audience_versions(audience_id, version) ON DELETE RESTRICT
		)`,
		`CREATE TABLE IF NOT EXISTS campaign_runs (
			id UUID PRIMARY KEY, campaign_id UUID NOT NULL, campaign_version INTEGER NOT NULL,
			audience_id UUID, audience_version INTEGER, audience_build_id UUID,
			status VARCHAR(24) NOT NULL CHECK (status IN ('snapshotting', 'dispatching', 'paused', 'completed', 'failed', 'cancelled')),
			run_seed VARCHAR(255) NOT NULL, snapshot_last_customer_id UUID,
			snapshot_count BIGINT NOT NULL DEFAULT 0 CHECK (snapshot_count >= 0), next_ordinal BIGINT NOT NULL DEFAULT 1 CHECK (next_ordinal > 0),
			started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (campaign_id, campaign_version) REFERENCES campaign_versions(campaign_id, version) ON DELETE RESTRICT,
			FOREIGN KEY (audience_id, audience_version) REFERENCES audience_versions(audience_id, version) ON DELETE RESTRICT,
			FOREIGN KEY (audience_build_id) REFERENCES audience_builds(id) ON DELETE RESTRICT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_campaign_runs_resume ON campaign_runs(status, updated_at) WHERE status IN ('snapshotting', 'dispatching')`,
		`CREATE INDEX IF NOT EXISTS idx_campaign_runs_audience_build ON campaign_runs(audience_build_id) WHERE audience_build_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS campaign_recipient_snapshots (
			run_id UUID NOT NULL REFERENCES campaign_runs(id) ON DELETE RESTRICT, ordinal BIGINT NOT NULL CHECK (ordinal > 0),
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT, variant VARCHAR(100) NOT NULL, source_build_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (run_id, ordinal), UNIQUE (run_id, customer_id, variant)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_campaign_snapshots_customer ON campaign_recipient_snapshots(customer_id, run_id)`,
		`CREATE TABLE IF NOT EXISTS frequency_policies (
			id UUID NOT NULL, version INTEGER NOT NULL CHECK (version > 0), name VARCHAR(255) NOT NULL,
			scope VARCHAR(24) NOT NULL CHECK (scope IN ('campaign', 'trigger', 'workspace_global')), scope_ref VARCHAR(255), channel VARCHAR(20) NOT NULL,
			max_events INTEGER NOT NULL CHECK (max_events > 0), window_kind VARCHAR(20) NOT NULL CHECK (window_kind IN ('sliding', 'calendar')),
			window_seconds BIGINT NOT NULL CHECK (window_seconds > 0), timezone VARCHAR(64),
			deny_action VARCHAR(20) NOT NULL CHECK (deny_action IN ('suppress', 'defer')), priority INTEGER NOT NULL DEFAULT 100,
			enabled BOOLEAN NOT NULL DEFAULT TRUE, created_by VARCHAR(255), created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id, version),
			CHECK ((scope = 'workspace_global' AND scope_ref IS NULL) OR (scope <> 'workspace_global' AND scope_ref IS NOT NULL)),
			CHECK ((window_kind = 'sliding') OR (window_kind = 'calendar' AND timezone IS NOT NULL))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_frequency_policies_resolve ON frequency_policies(enabled, scope, scope_ref, channel, priority)`,
		`CREATE TABLE IF NOT EXISTS frequency_decisions (
			id UUID PRIMARY KEY, reservation_id VARCHAR(255) NOT NULL UNIQUE, effect_key CHAR(64) NOT NULL,
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT, channel VARCHAR(20) NOT NULL,
			allowed BOOLEAN NOT NULL, deferred BOOLEAN NOT NULL DEFAULT FALSE, matched_scope VARCHAR(24),
			policy_versions JSONB NOT NULL, reason TEXT, decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (effect_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_frequency_decisions_customer ON frequency_decisions(customer_id, channel, decided_at DESC)`,
		`CREATE TABLE IF NOT EXISTS import_jobs (
			id UUID PRIMARY KEY, status VARCHAR(20) NOT NULL CHECK (status IN ('uploading', 'staged', 'processing', 'completed', 'rejected', 'cancelled')),
			filename VARCHAR(512) NOT NULL, object_key VARCHAR(1024), file_checksum CHAR(64), list_ids TEXT[] NOT NULL DEFAULT '{}'::text[],
			total_count BIGINT NOT NULL DEFAULT 0, pending_count BIGINT NOT NULL DEFAULT 0, processing_count BIGINT NOT NULL DEFAULT 0,
			succeeded_count BIGINT NOT NULL DEFAULT 0, failed_count BIGINT NOT NULL DEFAULT 0, rejection_reason TEXT, created_by VARCHAR(255),
			committed_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CHECK (total_count >= 0 AND pending_count >= 0 AND processing_count >= 0 AND succeeded_count >= 0 AND failed_count >= 0),
			CHECK (total_count = pending_count + processing_count + succeeded_count + failed_count)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_import_jobs_resume ON import_jobs(status, updated_at) WHERE status IN ('staged', 'processing')`,
		`CREATE TABLE IF NOT EXISTS import_job_rows (
			job_id UUID NOT NULL REFERENCES import_jobs(id) ON DELETE RESTRICT, ordinal BIGINT NOT NULL CHECK (ordinal > 0),
			raw_payload JSONB NOT NULL, row_checksum CHAR(64) NOT NULL,
			status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'processing', 'succeeded', 'failed')),
			claim_token UUID, lease_expires_at TIMESTAMPTZ, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			customer_id UUID REFERENCES customers(id) ON DELETE SET NULL, action VARCHAR(32), error_code VARCHAR(100), error_detail TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (job_id, ordinal), UNIQUE (job_id, row_checksum, ordinal)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_import_rows_claim ON import_job_rows(job_id, status, ordinal) WHERE status IN ('pending', 'processing')`,
		`CREATE TABLE IF NOT EXISTS import_job_checkpoints (
			job_id UUID PRIMARY KEY REFERENCES import_jobs(id) ON DELETE CASCADE,
			staged_ordinal BIGINT NOT NULL DEFAULT 0 CHECK (staged_ordinal >= 0), processed_ordinal BIGINT NOT NULL DEFAULT 0 CHECK (processed_ordinal >= 0),
			object_offset BIGINT NOT NULL DEFAULT 0 CHECK (object_offset >= 0), updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
}
