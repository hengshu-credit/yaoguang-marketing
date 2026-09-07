package schema

// ExecutionEventMetadataDefinitions enriches only server-emitted message events.
// The message row, rather than user supplied event properties, establishes the
// automation and touchpoint identity used for attribution and loop prevention.
func ExecutionEventMetadataDefinitions() []string {
	return []string{
		`CREATE OR REPLACE FUNCTION notifuse_execution_event_metadata() RETURNS TRIGGER AS $$
		DECLARE v_message RECORD; v_metadata JSONB; v_cause TEXT;
		BEGIN
		 IF NEW.source <> 'contact_timeline' OR NEW.subject_type <> 'message_history' THEN RETURN NEW; END IF;
		 SELECT id,automation_id,broadcast_id,message_data INTO v_message FROM message_history WHERE id=NEW.subject_id;
		 IF NOT FOUND THEN RETURN NEW; END IF;
		 v_metadata:=COALESCE(v_message.message_data->'metadata','{}'::jsonb);
		 NEW.properties:=COALESCE(NEW.properties,'{}'::jsonb) || jsonb_strip_nulls(jsonb_build_object(
		  'message_id',v_message.id,'touchpoint_id',v_message.id,'automation_id',v_message.automation_id,
		  'broadcast_id',v_message.broadcast_id,'execution_id',v_metadata->>'execution_id'));
		 v_cause:=COALESCE(v_metadata->>'causation_id',v_metadata->>'origin_event_id');
		 IF v_cause ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
		  NEW.context:=COALESCE(NEW.context,'{}'::jsonb)||jsonb_build_object('causation_id',v_cause);
		 END IF;
		 RETURN NEW;
		END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS execution_event_metadata ON event_ledger`,
		`CREATE TRIGGER execution_event_metadata BEFORE INSERT ON event_ledger FOR EACH ROW EXECUTE FUNCTION notifuse_execution_event_metadata()`,
		`CREATE OR REPLACE FUNCTION notifuse_execution_outbox_metadata() RETURNS TRIGGER AS $$
		DECLARE v_properties JSONB; v_context JSONB;
		BEGIN
		 IF NEW.payload->>'source' <> 'contact_timeline' OR NEW.payload->'subject'->>'type' <> 'message_history' THEN RETURN NEW; END IF;
		 SELECT properties,context INTO v_properties,v_context FROM event_ledger WHERE id=NEW.event_id AND source='contact_timeline' AND subject_type='message_history' LIMIT 1;
		 IF NOT FOUND THEN RETURN NEW; END IF;
		 NEW.payload:=jsonb_set(NEW.payload,'{data}',COALESCE(NEW.payload->'data','{}'::jsonb)||jsonb_strip_nulls(jsonb_build_object(
		  'message_id',v_properties->>'message_id','touchpoint_id',v_properties->>'touchpoint_id',
		  'automation_id',v_properties->>'automation_id','broadcast_id',v_properties->>'broadcast_id',
		  'execution_id',v_properties->>'execution_id')));
		 IF v_context->>'causation_id' IS NOT NULL THEN NEW.payload:=NEW.payload||jsonb_build_object('causation_id',v_context->>'causation_id'); END IF;
		 RETURN NEW;
		END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS execution_outbox_metadata ON event_outbox`,
		`CREATE TRIGGER execution_outbox_metadata BEFORE INSERT ON event_outbox FOR EACH ROW EXECUTE FUNCTION notifuse_execution_outbox_metadata()`,
	}
}
