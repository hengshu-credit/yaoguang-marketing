package schema

func SubjectStateTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS subject_state_projections (subject_type TEXT NOT NULL, subject_id TEXT NOT NULL, revision BIGINT NOT NULL DEFAULT 0, next_check TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY(subject_type,subject_id))`,
		`CREATE INDEX IF NOT EXISTS idx_subject_state_due ON subject_state_projections(next_check) WHERE next_check IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS subject_state_history (subject_type TEXT NOT NULL,subject_id TEXT NOT NULL,customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,state_key TEXT NOT NULL,value TEXT NOT NULL CHECK(value IN ('true','false','unknown')),valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,rule_revision INTEGER NOT NULL,projection_revision BIGINT NOT NULL,source TEXT NOT NULL,reason TEXT NOT NULL DEFAULT '',event_ids JSONB NOT NULL DEFAULT '[]',computed_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(subject_type,subject_id,state_key,projection_revision,valid_from),CHECK(valid_to IS NULL OR valid_to>valid_from))`,
		`CREATE INDEX IF NOT EXISTS idx_subject_state_history_customer ON subject_state_history(customer_id,state_key,valid_from)`,
		`CREATE TABLE IF NOT EXISTS subject_state_overrides (id BIGSERIAL PRIMARY KEY,subject_type TEXT NOT NULL,subject_id TEXT NOT NULL,state_key TEXT NOT NULL,value TEXT NOT NULL CHECK(value IN ('true','false','unknown')),reason TEXT NOT NULL CHECK(length(reason)>0),actor TEXT NOT NULL,valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),CHECK(valid_to IS NULL OR valid_to>valid_from))`,
		`CREATE INDEX IF NOT EXISTS idx_subject_state_overrides_subject ON subject_state_overrides(subject_type,subject_id,state_key,valid_from)`,
	}
}
