package schema

func FunnelTableDefinitions() []string {
	return []string{`CREATE TABLE IF NOT EXISTS funnel_query_jobs(id UUID PRIMARY KEY,status TEXT NOT NULL CHECK(status IN ('queued','running','completed','failed','cancelled')),query JSONB NOT NULL,result JSONB,error TEXT NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`, `CREATE INDEX IF NOT EXISTS idx_funnel_query_jobs_created ON funnel_query_jobs(created_at DESC)`}
}
