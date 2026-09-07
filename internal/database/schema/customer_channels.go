package schema

import "strings"

func CustomerChannelDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS customer_endpoint_consent_audit(id UUID PRIMARY KEY,customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,channel TEXT NOT NULL,granted BOOLEAN NOT NULL,actor_id TEXT,source TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`ALTER TABLE contact_timeline ALTER COLUMN email DROP NOT NULL`,
		`DO $$ DECLARE body TEXT; BEGIN SELECT pg_get_functiondef('queue_contact_for_segment_recomputation()'::regprocedure) INTO body;IF position('IF NEW.email IS NULL THEN RETURN NEW;' IN body)=0 THEN body:=regexp_replace(body,'BEGIN','BEGIN IF NEW.email IS NULL THEN RETURN NEW; END IF;');EXECUTE body;END IF;END;$$`,
		`ALTER TABLE contact_endpoints ALTER COLUMN email DROP NOT NULL`,
		`ALTER TABLE channel_send_executions ALTER COLUMN contact_email DROP NOT NULL`,
		`ALTER TABLE channel_send_executions ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE SET NULL`,
		`ALTER TABLE message_history ALTER COLUMN contact_email DROP NOT NULL`,
		`ALTER TABLE message_history ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE SET NULL`,
		`CREATE OR REPLACE FUNCTION notifuse_channel_customer_identity() RETURNS TRIGGER AS $$ BEGIN NEW.contact_email=NULLIF(NEW.contact_email,'');IF NEW.customer_id IS NULL THEN SELECT COALESCE(e.customer_id,c.customer_id) INTO NEW.customer_id FROM contact_endpoints e LEFT JOIN contacts c ON c.email=e.email WHERE e.endpoint_id=NEW.endpoint_id;END IF;RETURN NEW;END;$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS channel_customer_identity ON channel_send_executions`,
		`CREATE TRIGGER channel_customer_identity BEFORE INSERT ON channel_send_executions FOR EACH ROW EXECUTE FUNCTION notifuse_channel_customer_identity()`,
		strings.Replace(strings.Replace(ContactEndpointTimelineFunction(), "INSERT INTO contact_timeline (email, operation", "INSERT INTO contact_timeline (customer_id, email, operation", 1), "NEW.email, CASE WHEN TG_OP", "NEW.customer_id, NEW.email, CASE WHEN TG_OP", 1),
		`DO $$ DECLARE body TEXT; BEGIN SELECT pg_get_functiondef('track_message_history_changes()'::regprocedure) INTO body;IF position('IF NEW.contact_email IS NULL THEN RETURN NEW;' IN body)=0 THEN body:=regexp_replace(body,'BEGIN','BEGIN IF NEW.contact_email IS NULL THEN RETURN NEW; END IF;');EXECUTE body;END IF;END;$$`,
		`CREATE OR REPLACE FUNCTION notifuse_customer_message_timeline() RETURNS TRIGGER AS $$ DECLARE kind TEXT;BEGIN
 IF NEW.contact_email IS NOT NULL OR NEW.customer_id IS NULL THEN RETURN NEW;END IF;
 IF TG_OP='INSERT' THEN kind=NEW.channel||'.sent';
 ELSIF NEW.delivered_at IS DISTINCT FROM OLD.delivered_at AND NEW.delivered_at IS NOT NULL THEN kind=NEW.channel||'.delivered';
 ELSIF NEW.clicked_at IS DISTINCT FROM OLD.clicked_at AND NEW.clicked_at IS NOT NULL THEN kind=NEW.channel||'.clicked';
 ELSIF NEW.opened_at IS DISTINCT FROM OLD.opened_at AND NEW.opened_at IS NOT NULL THEN kind=NEW.channel||'.opened';
 ELSIF NEW.failed_at IS DISTINCT FROM OLD.failed_at AND NEW.failed_at IS NOT NULL THEN kind=NEW.channel||'.failed';
 ELSE RETURN NEW;END IF;
 INSERT INTO contact_timeline(customer_id,email,operation,entity_type,entity_id,kind,changes,created_at) VALUES(NEW.customer_id,NULL,lower(TG_OP),'message_history',NEW.id,kind,jsonb_build_object('message_id',NEW.id),CURRENT_TIMESTAMP);RETURN NEW;
 END;$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS customer_message_timeline ON message_history`,
		`CREATE TRIGGER customer_message_timeline AFTER INSERT OR UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION notifuse_customer_message_timeline()`,
	}
}
