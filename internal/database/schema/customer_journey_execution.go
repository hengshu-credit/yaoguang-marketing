package schema

import "strings"

// CustomerJourneyExecutionDefinitions upgrades the existing enrollment function
// without changing historical migrations or the legacy email adapter. Customer
// enrollment does not require a delivery address; channel nodes resolve one.
func CustomerJourneyExecutionDefinitions() []string {
	function := JourneyAutomationEnrollContactFunction()
	emailGuard := `IF NULLIF(BTRIM(p_contact_email), '') IS NULL THEN
				INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason)
				VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, 'guard_denied', 'contact_email_projection_missing');
				outcome := 'guard_denied'; RETURN NEXT; RETURN;
			END IF;`
	function = strings.Replace(function, emailGuard, "p_contact_email := NULLIF(BTRIM(p_contact_email), '');", 1)
	function = strings.Replace(function, "IF p_frequency = 'once' THEN\n\t\t\t\tINSERT INTO automation_trigger_log", "IF p_frequency = 'once' AND p_contact_email IS NOT NULL THEN\n\t\t\t\tINSERT INTO automation_trigger_log", 1)
	function = strings.Replace(function, "INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at, customer_id)", "IF p_contact_email IS NOT NULL THEN\n\t\t\tINSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at, customer_id)", 1)
	function = strings.Replace(function, "outcome := 'enrolled'; contact_automation_id := v_new_id;", "END IF;\n\t\t\tUPDATE contact_automations SET context=COALESCE(context,'{}'::jsonb)||jsonb_build_object('customer_id',v_customer_id) WHERE id=v_new_id;\n\t\t\toutcome := 'enrolled'; contact_automation_id := v_new_id;", 1)
	return []string{`ALTER TABLE contact_automations ALTER COLUMN contact_email DROP NOT NULL`, function}
}
