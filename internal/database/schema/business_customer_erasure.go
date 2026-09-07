package schema

func BusinessCustomerErasureDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS business_erased_sessions(session_hash TEXT PRIMARY KEY,erased_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE IF NOT EXISTS business_erased_reports(report_hash TEXT PRIMARY KEY,source_hash TEXT NOT NULL,payload_hash TEXT NOT NULL,erased_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE INDEX IF NOT EXISTS idx_business_erased_payload ON business_erased_reports(source_hash,payload_hash)`,
		`CREATE OR REPLACE FUNCTION notifuse_erase_business_customer_data() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO business_erased_sessions(session_hash) SELECT DISTINCT md5(source||':'||(normalized_event->'customer_attributes'->>'browser_session_id')) FROM event_report_receipts WHERE customer_id=OLD.id AND normalized_event->'customer_attributes'->>'browser_session_id' IS NOT NULL ON CONFLICT DO NOTHING;
 INSERT INTO business_erased_reports(report_hash,source_hash,payload_hash) SELECT md5(source||':'||report_id),md5(source),payload_hash FROM event_report_receipts WHERE customer_id=OLD.id OR md5(source||':'||(normalized_event->'customer_attributes'->>'browser_session_id')) IN(SELECT session_hash FROM business_erased_sessions) ON CONFLICT DO NOTHING;
 DELETE FROM event_outbox WHERE event_id IN(SELECT event_id FROM event_report_receipts WHERE md5(source||':'||report_id) IN(SELECT report_hash FROM business_erased_reports)) OR payload->'subject'->>'customer_id'=OLD.id::text;
 DELETE FROM event_ledger WHERE customer_id=OLD.id OR id IN(SELECT event_id FROM event_report_receipts WHERE md5(source||':'||report_id) IN(SELECT report_hash FROM business_erased_reports));
 DELETE FROM event_report_receipts WHERE customer_id=OLD.id OR md5(source||':'||report_id) IN(SELECT report_hash FROM business_erased_reports);
	DELETE FROM event_ingest_diagnostics WHERE md5(source||':'||report_id) IN(SELECT report_hash FROM business_erased_reports);
 DELETE FROM subject_state_projections WHERE (subject_type='customer' AND subject_id=OLD.id::text) OR (subject_type,subject_id) IN(SELECT object_type,id::text FROM business_objects WHERE customer_id=OLD.id);
 DELETE FROM subject_state_overrides WHERE (subject_type='customer' AND subject_id=OLD.id::text) OR (subject_type,subject_id) IN(SELECT object_type,id::text FROM business_objects WHERE customer_id=OLD.id);
 RETURN OLD;
 END $$`,
		`DROP TRIGGER IF EXISTS customer_business_data_erasure ON customers`,
		`CREATE TRIGGER customer_business_data_erasure BEFORE DELETE ON customers FOR EACH ROW EXECUTE FUNCTION notifuse_erase_business_customer_data()`,
		`CREATE OR REPLACE FUNCTION notifuse_erase_customer_execution_media() RETURNS TRIGGER LANGUAGE plpgsql AS $$
 DECLARE erased_hashes TEXT[];
 BEGIN
 IF to_regclass('execution_evidence') IS NULL THEN RETURN OLD; END IF;
 SELECT array_agg(DISTINCT asset->>'hash') INTO erased_hashes FROM execution_evidence e CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(e.payload->'assets')='array' THEN e.payload->'assets' ELSE '[]'::jsonb END) asset WHERE e.customer_id=OLD.id;
 DELETE FROM execution_evidence WHERE customer_id=OLD.id;
 DELETE FROM execution_archived_assets a WHERE a.hash::text=ANY(erased_hashes)
 AND NOT EXISTS(SELECT 1 FROM execution_evidence e CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(e.payload->'assets')='array' THEN e.payload->'assets' ELSE '[]'::jsonb END) asset WHERE asset->>'hash'=a.hash::text)
 AND NOT EXISTS(SELECT 1 FROM execution_config_snapshots s CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(s.materials->'assets')='array' THEN s.materials->'assets' ELSE '[]'::jsonb END) asset WHERE asset->>'hash'=a.hash::text)
 AND NOT EXISTS(SELECT 1 FROM execution_template_material_archives m CROSS JOIN LATERAL jsonb_array_elements(m.assets) asset WHERE asset->>'hash'=a.hash::text);
 RETURN OLD;
 END $$`,
		`DROP TRIGGER IF EXISTS customer_execution_media_erasure ON customers`,
		`CREATE TRIGGER customer_execution_media_erasure BEFORE DELETE ON customers FOR EACH ROW EXECUTE FUNCTION notifuse_erase_customer_execution_media()`,
	}
}
