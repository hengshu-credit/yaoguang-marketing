package schema

// ExecutionSnapshotTableDefinitions is shared by workspace initialization and
// upgrades. Each workspace connection owns its snapshots and archived bytes.
func ExecutionSnapshotTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS execution_provider_refresh(singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),pending BOOLEAN NOT NULL DEFAULT false)`,
		`CREATE TABLE IF NOT EXISTS execution_runtime_credentials(id UUID PRIMARY KEY,automation_id TEXT NOT NULL,configuration_hash CHAR(64) NOT NULL,encrypted_nodes TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(automation_id,configuration_hash))`,
		`ALTER TABLE execution_runtime_credentials ADD COLUMN IF NOT EXISTS includes_providers BOOLEAN NOT NULL DEFAULT false`,
		`DO $$ DECLARE entry RECORD; BEGIN FOR entry IN SELECT conname FROM pg_constraint WHERE conrelid='execution_runtime_credentials'::regclass AND contype='u' LOOP EXECUTE format('ALTER TABLE execution_runtime_credentials DROP CONSTRAINT %I',entry.conname); END LOOP; END $$`,
		`CREATE INDEX IF NOT EXISTS execution_runtime_config_latest ON execution_runtime_credentials(automation_id,configuration_hash,created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS execution_template_material_archives(template_id TEXT NOT NULL,content_hash CHAR(64) NOT NULL,content JSONB NOT NULL,assets JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(template_id,content_hash))`,
		`CREATE TABLE IF NOT EXISTS execution_config_snapshots (
 id UUID PRIMARY KEY, execution_id TEXT NOT NULL UNIQUE, kind TEXT NOT NULL CHECK(kind IN ('campaign','journey','decision')),
 source_id TEXT NOT NULL DEFAULT '', configuration JSONB NOT NULL, materials JSONB NOT NULL,
 dependencies JSONB NOT NULL DEFAULT '{}'::jsonb, content_hash CHAR(64) NOT NULL,
 completeness TEXT NOT NULL CHECK(completeness IN ('complete','partial','legacy')), missing JSONB NOT NULL DEFAULT '[]'::jsonb,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_snapshots_source ON execution_config_snapshots(source_id,created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS execution_evidence (
 id UUID PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES execution_config_snapshots(execution_id) ON DELETE CASCADE,
 evidence_key TEXT NOT NULL, kind TEXT NOT NULL, node_id TEXT NOT NULL DEFAULT '', customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
 delivery_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, payload JSONB NOT NULL, payload_hash CHAR(64) NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(execution_id,evidence_key))`,
		`CREATE INDEX IF NOT EXISTS idx_execution_evidence_timeline ON execution_evidence(execution_id,occurred_at,id)`,
		`CREATE TABLE IF NOT EXISTS execution_archived_assets (hash CHAR(64) PRIMARY KEY, media_type TEXT NOT NULL, data BYTEA NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE OR REPLACE FUNCTION notifuse_reject_execution_update() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'Execution history is immutable'; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS execution_credentials_immutable ON execution_runtime_credentials`,
		`CREATE TRIGGER execution_credentials_immutable BEFORE UPDATE ON execution_runtime_credentials FOR EACH ROW EXECUTE FUNCTION notifuse_reject_execution_update()`,
		`DROP TRIGGER IF EXISTS execution_snapshot_immutable ON execution_config_snapshots`,
		`CREATE TRIGGER execution_snapshot_immutable BEFORE UPDATE ON execution_config_snapshots FOR EACH ROW EXECUTE FUNCTION notifuse_reject_execution_update()`,
		`DROP TRIGGER IF EXISTS execution_evidence_immutable ON execution_evidence`,
		`CREATE TRIGGER execution_evidence_immutable BEFORE UPDATE ON execution_evidence FOR EACH ROW EXECUTE FUNCTION notifuse_reject_execution_update()`,
		`DROP TRIGGER IF EXISTS execution_manifest_immutable ON execution_template_material_archives`,
		`CREATE TRIGGER execution_manifest_immutable BEFORE UPDATE ON execution_template_material_archives FOR EACH ROW EXECUTE FUNCTION notifuse_reject_execution_update()`,
		`DROP TRIGGER IF EXISTS execution_asset_immutable ON execution_archived_assets`,
		`CREATE TRIGGER execution_asset_immutable BEFORE UPDATE ON execution_archived_assets FOR EACH ROW EXECUTE FUNCTION notifuse_reject_execution_update()`,
		`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS current_configuration JSONB`,
		`ALTER TABLE campaign_runs ALTER COLUMN campaign_version DROP NOT NULL`,
		`ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS execution_configuration JSONB`,
	}
}
