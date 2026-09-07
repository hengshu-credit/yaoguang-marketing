package schema

// ExecutionRuntimeSnapshotDefinitions captures enrollment and delivery evidence
// inside the existing transactions, covering trigger-driven enrollments too.
func ExecutionRuntimeSnapshotDefinitions() []string {
	return []string{
		`CREATE OR REPLACE FUNCTION notifuse_redact_execution_json(value JSONB) RETURNS JSONB AS $$
DECLARE result JSONB; entry RECORD;
BEGIN
 IF jsonb_typeof(value)='object' THEN
  result='{}'::jsonb;
  FOR entry IN SELECT key,val FROM jsonb_each(value) AS x(key,val) LOOP
   IF regexp_replace(lower(entry.key),'[-_ ]','','g') IN ('authorization','apikey','secret','clientsecret','password','accesstoken','refreshtoken','privatekey','jwtsecret','hmacsecret','cookie','setcookie','providersettings','authtoken','encryptedauthtoken','apikeysecret','encryptedapikeysecret','signingsecret') THEN result=result||jsonb_build_object(entry.key,'[REDACTED]');
   ELSE result=result||jsonb_build_object(entry.key,notifuse_redact_execution_json(entry.val)); END IF;
  END LOOP; RETURN result;
 ELSIF jsonb_typeof(value)='array' THEN
  SELECT COALESCE(jsonb_agg(notifuse_redact_execution_json(v)),'[]'::jsonb) INTO result FROM jsonb_array_elements(value) v;RETURN result;
 ELSE RETURN value; END IF;
END; $$ LANGUAGE plpgsql IMMUTABLE`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_journey_snapshot() RETURNS TRIGGER AS $$
DECLARE config JSONB; materials JSONB; missing JSONB='[]'::jsonb; source TEXT; asset_refs JSONB='[]'::jsonb; material_template JSONB; material_content JSONB; archive JSONB; dependencies JSONB; runtime_configs JSONB; runtime_ref UUID; requires_providers BOOLEAN;
BEGIN
 PERFORM pg_advisory_xact_lock_shared(hashtext('execution-provider-refresh'));
 SELECT to_jsonb(a),a.id INTO config,source FROM journey_enrollments e JOIN automations a ON a.id=e.automation_id WHERE e.id=NEW.enrollment_id;
 IF config IS NULL THEN RAISE EXCEPTION 'Journey configuration is missing'; END IF;
 SELECT jsonb_object_agg(n->>'id',n->'config') INTO runtime_configs FROM jsonb_array_elements(config->'nodes')n;
 SELECT EXISTS(SELECT 1 FROM jsonb_array_elements(config->'nodes') n WHERE n->>'type'='email' OR COALESCE(n->'config'->>'integration_id','')<>'') INTO requires_providers;
 IF requires_providers AND EXISTS(SELECT 1 FROM execution_provider_refresh WHERE singleton AND pending) THEN RAISE EXCEPTION 'Execution providers are awaiting preparation'; END IF;
 IF requires_providers OR runtime_configs IS DISTINCT FROM notifuse_redact_execution_json(runtime_configs) THEN
 SELECT id INTO runtime_ref FROM execution_runtime_credentials WHERE automation_id=source AND configuration_hash=encode(sha256(convert_to(runtime_configs::text,'UTF8')),'hex') AND (NOT requires_providers OR includes_providers) ORDER BY created_at DESC,id DESC LIMIT 1;
 IF runtime_ref IS NULL THEN RAISE EXCEPTION 'Runtime credentials must be archived before journey enrollment'; END IF;
 END IF;
 SELECT COALESCE(jsonb_agg(t.body),'[]'::jsonb) INTO materials FROM (
 SELECT DISTINCT ON (template.id, template.version) to_jsonb(template) AS body FROM jsonb_array_elements(COALESCE(config->'nodes','[]'::jsonb)) node
 JOIN LATERAL (SELECT t.* FROM templates t WHERE t.id=node->'config'->>'template_id' AND t.deleted_at IS NULL AND (COALESCE((node->'config'->>'template_version')::bigint,0)=0 OR t.version=(node->'config'->>'template_version')::bigint) ORDER BY t.version DESC LIMIT 1) template ON TRUE
 )t;
 IF EXISTS(SELECT 1 FROM jsonb_array_elements(COALESCE(config->'nodes','[]'::jsonb)) node WHERE node->'config'->>'template_id' IS NOT NULL AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(materials) t WHERE t->>'id'=node->'config'->>'template_id' AND (COALESCE((node->'config'->>'template_version')::bigint,0)=0 OR (t->>'version')::bigint=(node->'config'->>'template_version')::bigint))) THEN RAISE EXCEPTION 'Journey template snapshot is incomplete'; END IF;
 FOR material_template IN SELECT value FROM jsonb_array_elements(materials) LOOP
  material_content=notifuse_redact_execution_json(jsonb_build_object('channel',material_template->'channel','email',material_template->'email','web',material_template->'web','sms',material_template->'sms','push',material_template->'push','content',material_template->'content','translations',COALESCE(NULLIF(material_template->'translations','null'::jsonb),'{}'::jsonb),'settings',material_template->'settings','template_macro_id',material_template->'template_macro_id'));
  SELECT a.assets INTO archive FROM execution_template_material_archives a WHERE a.template_id=material_template->>'id' AND a.content=material_content;
  IF archive IS NULL AND material_content::text ~* '(https?://|src[[:space:]]*=)' THEN RAISE EXCEPTION 'Template media must be archived before journey enrollment: %',material_template->>'id'; END IF;
  asset_refs=asset_refs||COALESCE(archive,'[]'::jsonb);
 END LOOP;
 SELECT COALESCE(jsonb_agg(to_jsonb(d)),'[]'::jsonb) INTO dependencies FROM business_definitions d WHERE status='active';
 config=notifuse_redact_execution_json(config);materials=notifuse_redact_execution_json(jsonb_build_object('templates',materials,'assets',asset_refs));
 INSERT INTO execution_config_snapshots(id,execution_id,kind,source_id,configuration,materials,dependencies,content_hash,completeness,missing,created_at)
 VALUES(gen_random_uuid(),NEW.id::text,'journey',source,config,materials,jsonb_build_object('runtime_nodes_ref',runtime_ref,'business_definitions',notifuse_redact_execution_json(dependencies),'enrollment',(SELECT notifuse_redact_execution_json(to_jsonb(e)) FROM journey_enrollments e WHERE e.id=NEW.enrollment_id)),encode(sha256(convert_to(jsonb_build_array(config,materials)::text,'UTF8')),'hex'),CASE WHEN missing='[]'::jsonb THEN 'complete' ELSE 'partial' END,missing,NEW.started_at);
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS journey_execution_snapshot ON journey_instances`,
		`CREATE TRIGGER journey_execution_snapshot AFTER INSERT ON journey_instances FOR EACH ROW EXECUTE FUNCTION notifuse_capture_journey_snapshot()`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_journey_evidence() RETURNS TRIGGER AS $$
DECLARE execution TEXT; body JSONB; customer UUID;
BEGIN
 SELECT i.id::text,e.customer_id INTO execution,customer FROM journey_instances i JOIN journey_enrollments e ON e.id=i.enrollment_id WHERE i.contact_automation_id=NEW.contact_automation_id;
 IF NOT EXISTS(SELECT 1 FROM execution_config_snapshots WHERE execution_id=execution) THEN RETURN NEW; END IF;
 body=notifuse_redact_execution_json(to_jsonb(NEW));
 INSERT INTO execution_evidence(id,execution_id,evidence_key,kind,node_id,customer_id,status,payload,payload_hash,occurred_at)
 VALUES(gen_random_uuid(),execution,'node:'||NEW.id||':'||NEW.action,'node',NEW.node_id,customer,NEW.action,body,encode(sha256(convert_to(body::text,'UTF8')),'hex'),COALESCE(NEW.completed_at,NEW.entered_at)) ON CONFLICT(execution_id,evidence_key) DO NOTHING;
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS journey_execution_evidence ON automation_node_executions`,
		`CREATE TRIGGER journey_execution_evidence AFTER INSERT OR UPDATE OF action,output,error ON automation_node_executions FOR EACH ROW EXECUTE FUNCTION notifuse_capture_journey_evidence()`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_delivery_content() RETURNS TRIGGER AS $$
DECLARE execution TEXT; body JSONB; customer UUID;
BEGIN
 execution=NEW.payload->>'execution_id';
 IF execution IS NULL OR execution='' THEN RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM execution_config_snapshots WHERE execution_id=execution) THEN RAISE EXCEPTION 'Delivery execution snapshot is missing'; END IF;
 SELECT c.customer_id INTO customer FROM contacts c WHERE lower(c.email)=lower(NEW.contact_email) LIMIT 1;
 body=notifuse_redact_execution_json(jsonb_build_object('queue_id',NEW.id,'message_id',NEW.message_id,'touchpoint_id',NEW.message_id,'execution_id',execution,'template_id',NEW.template_id,'contact_email',NEW.contact_email,'payload',NEW.payload-'provider_settings','attempts',NEW.attempts,'last_error',NEW.last_error));
 INSERT INTO execution_evidence(id,execution_id,evidence_key,kind,customer_id,delivery_id,status,payload,payload_hash,occurred_at)
 VALUES(gen_random_uuid(),execution,'delivery:'||NEW.id||':'||NEW.status||':'||NEW.attempts,CASE WHEN TG_OP='INSERT' THEN 'delivery_content' ELSE 'delivery_status' END,customer,COALESCE(NEW.delivery_intent_id::text,NEW.message_id),NEW.status,body,encode(sha256(convert_to(body::text,'UTF8')),'hex'),CURRENT_TIMESTAMP) ON CONFLICT(execution_id,evidence_key) DO NOTHING;
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS execution_delivery_content ON email_queue`,
		`CREATE TRIGGER execution_delivery_content AFTER INSERT OR UPDATE OF status,attempts ON email_queue FOR EACH ROW EXECUTE FUNCTION notifuse_capture_delivery_content()`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_execution_audience() RETURNS TRIGGER AS $$
DECLARE body JSONB;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM execution_config_snapshots WHERE execution_id=NEW.run_id::text) THEN RETURN NEW; END IF;
 body=jsonb_build_object('result','selected','recipient',to_jsonb(NEW));
 INSERT INTO execution_evidence(id,execution_id,evidence_key,kind,customer_id,status,payload,payload_hash,occurred_at)
 VALUES(gen_random_uuid(),NEW.run_id::text,'audience:'||NEW.ordinal,'audience',NEW.customer_id,'selected',body,encode(sha256(convert_to(body::text,'UTF8')),'hex'),NEW.created_at) ON CONFLICT(execution_id,evidence_key) DO NOTHING;
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS execution_audience ON campaign_recipient_snapshots`,
		`CREATE TRIGGER execution_audience AFTER INSERT ON campaign_recipient_snapshots FOR EACH ROW EXECUTE FUNCTION notifuse_capture_execution_audience()`,
		`CREATE OR REPLACE FUNCTION notifuse_capture_execution_eligibility() RETURNS TRIGGER AS $$
DECLARE body JSONB; execution TEXT; evidence_kind TEXT;
BEGIN
 execution=NEW.metadata->>'execution_id';
 IF execution IS NULL OR NOT EXISTS(SELECT 1 FROM execution_config_snapshots WHERE execution_id=execution) THEN RETURN NEW; END IF;
 evidence_kind=CASE WHEN NEW.status IN ('planned','reserved','suppressed','deferred') THEN 'eligibility' ELSE 'delivery_status' END;
 body=notifuse_redact_execution_json(to_jsonb(NEW)||jsonb_build_object('execution_id',execution,'message_id',NEW.metadata->>'touchpoint_id','touchpoint_id',NEW.metadata->>'touchpoint_id','frequency_decision',(SELECT to_jsonb(f) FROM frequency_decisions f WHERE f.effect_key=NEW.effect_key)));
 INSERT INTO execution_evidence(id,execution_id,evidence_key,kind,customer_id,delivery_id,status,payload,payload_hash,occurred_at)
 VALUES(gen_random_uuid(),execution,'intent:'||NEW.id||':'||NEW.status,evidence_kind,NEW.customer_id,NEW.id::text,NEW.status,body,encode(sha256(convert_to(body::text,'UTF8')),'hex'),CURRENT_TIMESTAMP) ON CONFLICT(execution_id,evidence_key) DO NOTHING;
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS execution_eligibility ON delivery_intents`,
		`CREATE TRIGGER execution_eligibility AFTER INSERT OR UPDATE OF status ON delivery_intents FOR EACH ROW EXECUTE FUNCTION notifuse_capture_execution_eligibility()`,
	}
}
