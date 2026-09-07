package schema

func TrackedEventTableDefinitions() []string {
	return append([]string{
		`CREATE TABLE IF NOT EXISTS event_report_receipts (source TEXT NOT NULL,report_id TEXT NOT NULL,event_id UUID NOT NULL UNIQUE,payload_hash TEXT NOT NULL,customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,reported_at TIMESTAMPTZ NOT NULL,processing_status TEXT NOT NULL,normalized_event JSONB NOT NULL,response JSONB NOT NULL DEFAULT '{}',PRIMARY KEY(source,report_id))`,
		`CREATE INDEX IF NOT EXISTS idx_event_receipts_customer ON event_report_receipts(customer_id,reported_at DESC)`,
		`ALTER TABLE event_report_receipts ADD COLUMN IF NOT EXISTS definition_snapshot JSONB NOT NULL DEFAULT '{}', ADD COLUMN IF NOT EXISTS mapping_snapshot JSONB, ADD COLUMN IF NOT EXISTS attribute_definitions JSONB NOT NULL DEFAULT '{}', ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ, ADD COLUMN IF NOT EXISTS resolution_reason TEXT`,
		`ALTER TABLE event_report_receipts ADD COLUMN IF NOT EXISTS object_definition_snapshot JSONB`,
		`CREATE TABLE IF NOT EXISTS event_ingest_diagnostics(id UUID PRIMARY KEY,source TEXT NOT NULL,report_id TEXT,event_code TEXT,error_code TEXT NOT NULL,message TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	}, BusinessCustomerErasureDefinitions()...)
}
