package schema

func ConfigurableRuntimeTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS configured_event_jobs(message_id UUID PRIMARY KEY REFERENCES event_outbox(id) ON DELETE CASCADE,customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),claim_token UUID,claim_until TIMESTAMPTZ,attempts INTEGER NOT NULL DEFAULT 0,completed_at TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_configured_event_jobs_due ON configured_event_jobs(available_at) WHERE completed_at IS NULL`,
		`CREATE TABLE IF NOT EXISTS marketing_trigger_subject_entries(automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,subject_type TEXT NOT NULL,subject_id TEXT NOT NULL,customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,event_id UUID NOT NULL,contact_automation_id TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),PRIMARY KEY(automation_id,subject_type,subject_id))`,
		`CREATE TABLE IF NOT EXISTS journey_event_subjects(contact_automation_id TEXT PRIMARY KEY,subject_type TEXT NOT NULL,subject_id TEXT NOT NULL,customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,origin_event_id UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE OR REPLACE FUNCTION notifuse_schedule_configured_event() RETURNS TRIGGER AS $$ BEGIN
 IF NEW.topic='notifuse.events' THEN
 INSERT INTO configured_event_jobs(message_id,customer_id) VALUES(NEW.id,COALESCE(NULLIF(NEW.payload->'subject'->>'customer_id','')::uuid,(SELECT customer_id FROM contacts WHERE email=NEW.payload->'subject'->>'contact_email'))) ON CONFLICT(message_id) DO NOTHING;
 END IF; RETURN NEW;END;$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS configured_event_job ON event_outbox`,
		`CREATE TRIGGER configured_event_job AFTER INSERT ON event_outbox FOR EACH ROW EXECUTE FUNCTION notifuse_schedule_configured_event()`,
	}
}
