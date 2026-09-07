package schema

// Full immutable snapshots allow event-time rules to read actual attribute evidence.
func SubjectAttributeHistoryTableDefinitions() []string {
	return append([]string{
		`CREATE TABLE IF NOT EXISTS subject_attribute_history(id BIGSERIAL PRIMARY KEY,subject_type TEXT NOT NULL,subject_id TEXT NOT NULL,customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,valid_from TIMESTAMPTZ NOT NULL,recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),event_id UUID,attributes JSONB NOT NULL,changed_keys JSONB)`,
		`CREATE INDEX IF NOT EXISTS idx_subject_attribute_history_time ON subject_attribute_history(subject_type,subject_id,valid_from,id)`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_subject_attributes() RETURNS TRIGGER AS $$
 DECLARE typ TEXT; sid TEXT; customer UUID; effective TIMESTAMPTZ; cause UUID; changed JSONB;
 BEGIN
 IF TG_OP='UPDATE' AND NEW.attributes IS NOT DISTINCT FROM OLD.attributes THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME='customer_profiles' THEN typ='customer';sid=NEW.customer_id::text;customer=NEW.customer_id;
 ELSE typ=NEW.object_type;sid=NEW.id::text;customer=NEW.customer_id; END IF;
 effective=COALESCE(NULLIF(current_setting('notifuse.business_event_at',true),'')::timestamptz,NOW());
 cause=NULLIF(current_setting('notifuse.business_event_id',true),'')::uuid;
 IF TG_OP='UPDATE' THEN SELECT COALESCE(jsonb_agg(k),'[]'::jsonb) INTO changed FROM jsonb_object_keys(NEW.attributes || OLD.attributes) k WHERE NEW.attributes->k IS DISTINCT FROM OLD.attributes->k; END IF;
 INSERT INTO subject_attribute_history(subject_type,subject_id,customer_id,valid_from,event_id,attributes,changed_keys) VALUES(typ,sid,customer,effective,cause,NEW.attributes,changed);
 INSERT INTO subject_state_projections(subject_type,subject_id,next_check) VALUES(typ,sid,NOW()) ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=NOW();
 RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS customer_state_attribute_history ON customer_profiles`,
		`CREATE TRIGGER customer_state_attribute_history AFTER INSERT OR UPDATE OF attributes ON customer_profiles FOR EACH ROW EXECUTE FUNCTION notifuse_capture_subject_attributes()`,
		`DROP TRIGGER IF EXISTS object_state_attribute_history ON business_objects`,
		`CREATE TRIGGER object_state_attribute_history AFTER INSERT OR UPDATE OF attributes ON business_objects FOR EACH ROW EXECUTE FUNCTION notifuse_capture_subject_attributes()`,
		`INSERT INTO subject_attribute_history(subject_type,subject_id,customer_id,valid_from,attributes) SELECT 'customer',customer_id::text,customer_id,NOW(),attributes FROM customer_profiles p WHERE NOT EXISTS(SELECT 1 FROM subject_attribute_history h WHERE h.subject_type='customer' AND h.subject_id=p.customer_id::text)`,
		`INSERT INTO subject_attribute_history(subject_type,subject_id,customer_id,valid_from,attributes) SELECT object_type,id::text,customer_id,NOW(),attributes FROM business_objects o WHERE NOT EXISTS(SELECT 1 FROM subject_attribute_history h WHERE h.subject_type=o.object_type AND h.subject_id=o.id::text)`,
	}, SubjectRelationHistoryTableDefinitions()...)
}
