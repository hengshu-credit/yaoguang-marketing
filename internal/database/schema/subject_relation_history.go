package schema

func SubjectRelationHistoryTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS subject_relation_history(id BIGSERIAL PRIMARY KEY,relation_id UUID NOT NULL,relation_key TEXT NOT NULL,from_type TEXT NOT NULL,from_id TEXT NOT NULL,to_type TEXT NOT NULL,to_id TEXT NOT NULL,valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`,
		`CREATE INDEX IF NOT EXISTS idx_subject_relation_history_watermark ON subject_relation_history(relation_id,recorded_at DESC,id DESC)`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_subject_relation() RETURNS TRIGGER AS $$
 DECLARE rel RECORD; a RECORD; b RECORD;
 BEGIN
 IF TG_TABLE_NAME='business_objects' THEN
  IF TG_OP='UPDATE' AND NEW.customer_id IS NOT DISTINCT FROM OLD.customer_id THEN RETURN NEW; END IF;
  INSERT INTO subject_relation_history(relation_id,relation_key,from_type,from_id,to_type,to_id,valid_from,valid_to) VALUES(NEW.id,'__customer_ownership','customer',NEW.customer_id::text,NEW.object_type,NEW.id::text,CASE WHEN TG_OP='INSERT' THEN COALESCE(NULLIF(current_setting('notifuse.business_event_at',true),'')::timestamptz,NEW.created_at) ELSE COALESCE((SELECT MIN(valid_from) FROM subject_relation_history WHERE relation_id=NEW.id AND relation_key='__customer_ownership'),NEW.created_at) END,NULL);
  RETURN NEW;
 END IF;
 IF TG_OP='DELETE' THEN rel=OLD; ELSE rel=NEW; END IF;
 SELECT object_type,customer_id INTO a FROM business_objects WHERE id=rel.from_id;
 SELECT object_type,customer_id INTO b FROM business_objects WHERE id=rel.to_id;
 IF a.object_type IS NULL OR b.object_type IS NULL THEN RETURN NULL; END IF;
 IF a.customer_id IS DISTINCT FROM b.customer_id THEN RAISE EXCEPTION 'Related objects must belong to the same customer'; END IF;
 INSERT INTO subject_relation_history(relation_id,relation_key,from_type,from_id,to_type,to_id,valid_from,valid_to) VALUES(rel.id,rel.relation_key,a.object_type,rel.from_id::text,b.object_type,rel.to_id::text,rel.valid_from,CASE WHEN TG_OP='DELETE' THEN COALESCE(rel.valid_to,clock_timestamp()) ELSE rel.valid_to END);
 INSERT INTO subject_state_projections(subject_type,subject_id,next_check) VALUES(a.object_type,rel.from_id::text,clock_timestamp()) ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=clock_timestamp();
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
 END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS object_relation_history ON business_object_relations`,
		`CREATE TRIGGER object_relation_history AFTER INSERT OR UPDATE OR DELETE ON business_object_relations FOR EACH ROW EXECUTE FUNCTION notifuse_capture_subject_relation()`,
		`DROP TRIGGER IF EXISTS customer_object_ownership_history ON business_objects`,
		`CREATE TRIGGER customer_object_ownership_history AFTER INSERT OR UPDATE OF customer_id ON business_objects FOR EACH ROW EXECUTE FUNCTION notifuse_capture_subject_relation()`,
		`INSERT INTO subject_relation_history(relation_id,relation_key,from_type,from_id,to_type,to_id,valid_from,valid_to) SELECT rel.id,rel.relation_key,a.object_type,rel.from_id::text,b.object_type,rel.to_id::text,rel.valid_from,rel.valid_to FROM business_object_relations rel JOIN business_objects a ON a.id=rel.from_id JOIN business_objects b ON b.id=rel.to_id WHERE NOT EXISTS(SELECT 1 FROM subject_relation_history h WHERE h.relation_id=rel.id)`,
		`INSERT INTO subject_relation_history(relation_id,relation_key,from_type,from_id,to_type,to_id,valid_from) SELECT o.id,'__customer_ownership','customer',o.customer_id::text,o.object_type,o.id::text,o.created_at FROM business_objects o WHERE NOT EXISTS(SELECT 1 FROM subject_relation_history h WHERE h.relation_id=o.id)`}
}
