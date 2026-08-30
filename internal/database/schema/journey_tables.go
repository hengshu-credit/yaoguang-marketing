package schema

// JourneyTableDefinitions establishes Customer/Event as the authority for
// Automation enrollment while retaining the legacy Email projection.
func JourneyTableDefinitions() []string {
	return []string{
		`ALTER TABLE contact_automations ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE RESTRICT`,
		`ALTER TABLE automation_trigger_log ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE RESTRICT`,
		`ALTER TABLE automation_trigger_bindings ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE RESTRICT`,
		`CREATE INDEX IF NOT EXISTS idx_contact_automations_customer ON contact_automations(customer_id, entered_at DESC) WHERE customer_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_automation_trigger_log_customer ON automation_trigger_log(automation_id, customer_id) WHERE customer_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS journey_enrollments (
			id UUID PRIMARY KEY,
			automation_id VARCHAR(36) NOT NULL REFERENCES automations(id) ON DELETE RESTRICT,
			automation_version INTEGER NOT NULL CHECK (automation_version > 0),
			customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
			contact_email VARCHAR(255),
			frequency VARCHAR(20) NOT NULL CHECK (frequency IN ('once', 'every_time')),
			origin_event_id UUID,
			dedupe_key CHAR(64) NOT NULL,
			entry_guard JSONB NOT NULL DEFAULT '{"enabled":false}'::jsonb,
			entered_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CHECK ((frequency = 'once' AND origin_event_id IS NULL) OR (frequency = 'every_time' AND origin_event_id IS NOT NULL))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_journey_enrollments_once ON journey_enrollments(automation_id, customer_id) WHERE frequency = 'once'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_journey_enrollments_event ON journey_enrollments(automation_id, customer_id, origin_event_id) WHERE frequency = 'every_time'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_journey_enrollments_dedupe ON journey_enrollments(dedupe_key)`,
		`CREATE INDEX IF NOT EXISTS idx_journey_enrollments_customer ON journey_enrollments(customer_id, entered_at DESC)`,
		`CREATE TABLE IF NOT EXISTS journey_instances (
			id UUID PRIMARY KEY,
			enrollment_id UUID NOT NULL UNIQUE REFERENCES journey_enrollments(id) ON DELETE RESTRICT,
			contact_automation_id VARCHAR(36) UNIQUE REFERENCES contact_automations(id) ON DELETE RESTRICT,
			status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'completed', 'exited', 'failed')),
			current_node_id VARCHAR(36), waiting_reason TEXT, next_scheduled_at TIMESTAMPTZ,
			started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_journey_instances_status_schedule ON journey_instances(status, next_scheduled_at) WHERE status = 'active'`,
		`CREATE TABLE IF NOT EXISTS journey_instance_events (
			id UUID PRIMARY KEY, journey_instance_id UUID NOT NULL REFERENCES journey_instances(id) ON DELETE CASCADE,
			node_id VARCHAR(36), event_type VARCHAR(32) NOT NULL,
			status VARCHAR(20) NOT NULL, reason TEXT, payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_journey_instance_events_trace ON journey_instance_events(journey_instance_id, occurred_at, id)`,
		`CREATE TABLE IF NOT EXISTS journey_entry_decisions (
			id UUID PRIMARY KEY, automation_id VARCHAR(36) NOT NULL, customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
			origin_event_id UUID, decision VARCHAR(24) NOT NULL CHECK (decision IN ('enrolled', 'already_once', 'replayed_event', 'guard_deferred', 'guard_denied', 'identity_unresolved')),
			reason TEXT, retry_at TIMESTAMPTZ, metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_journey_entry_decisions_customer ON journey_entry_decisions(customer_id, decided_at DESC)`,
		`CREATE TABLE IF NOT EXISTS journey_identity_reconciliation (
			automation_id VARCHAR(36) NOT NULL, contact_email VARCHAR(255) NOT NULL, origin_event_id UUID,
			reason VARCHAR(64) NOT NULL, first_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, occurrences BIGINT NOT NULL DEFAULT 1,
			PRIMARY KEY (automation_id, contact_email)
		)`,
	}
}

// JourneyAutomationEnrollContactFunction keeps the installed four-argument
// trigger ABI (plus the optional event id) but resolves Email to Customer before
// creating any new enrollment. Database constraints decide replay outcomes.
func JourneyAutomationEnrollContactFunction() string {
	return `CREATE OR REPLACE FUNCTION automation_enroll_contact(
			p_automation_id VARCHAR(36),
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_frequency VARCHAR(20),
			p_origin_event_id UUID DEFAULT NULL
		) RETURNS VOID AS $$
		DECLARE
			v_customer_id UUID;
			v_enrollment_id UUID;
			v_new_id VARCHAR(36);
			v_automation_version INTEGER;
			v_dedupe_key TEXT;
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM automations WHERE id = p_automation_id AND status = 'live' AND deleted_at IS NULL) THEN RETURN; END IF;

			SELECT customer_id INTO v_customer_id FROM contacts WHERE email = p_contact_email;
			IF v_customer_id IS NULL THEN
				INSERT INTO journey_identity_reconciliation (automation_id, contact_email, origin_event_id, reason)
				VALUES (p_automation_id, p_contact_email, p_origin_event_id, 'customer_not_resolved')
				ON CONFLICT (automation_id, contact_email) DO UPDATE SET last_seen_at = CURRENT_TIMESTAMP,
					occurrences = journey_identity_reconciliation.occurrences + 1, origin_event_id = EXCLUDED.origin_event_id;
				RETURN;
			END IF;

			SELECT version INTO v_automation_version FROM automations WHERE id = p_automation_id;
			IF p_frequency = 'once' THEN
				v_dedupe_key := encode(sha256(convert_to(concat_ws(':', p_automation_id, v_customer_id::text, 'once'), 'UTF8')), 'hex');
				INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, dedupe_key)
				VALUES (gen_random_uuid(), p_automation_id, COALESCE(v_automation_version, 1), v_customer_id, p_contact_email, 'once', v_dedupe_key)
				ON CONFLICT DO NOTHING RETURNING id INTO v_enrollment_id;
			ELSIF p_frequency = 'every_time' AND p_origin_event_id IS NOT NULL THEN
				v_dedupe_key := encode(sha256(convert_to(concat_ws(':', p_automation_id, v_customer_id::text, 'every_time', p_origin_event_id::text), 'UTF8')), 'hex');
				INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, origin_event_id, dedupe_key)
				VALUES (gen_random_uuid(), p_automation_id, COALESCE(v_automation_version, 1), v_customer_id, p_contact_email, 'every_time', p_origin_event_id, v_dedupe_key)
				ON CONFLICT DO NOTHING RETURNING id INTO v_enrollment_id;
			ELSE
				RETURN;
			END IF;

			IF v_enrollment_id IS NULL THEN
				INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason)
				VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id,
					CASE WHEN p_frequency = 'once' THEN 'already_once' ELSE 'replayed_event' END, 'database_unique_constraint');
				RETURN;
			END IF;

			v_new_id := gen_random_uuid()::text;
			INSERT INTO contact_automations (id, automation_id, contact_email, customer_id, current_node_id, status, entered_at, scheduled_at, origin_event_id, automation_version)
			VALUES (v_new_id, p_automation_id, p_contact_email, v_customer_id, p_root_node_id, 'active', NOW(), NOW(), p_origin_event_id, COALESCE(v_automation_version, 1));
			INSERT INTO journey_instances (id, enrollment_id, contact_automation_id, status, current_node_id)
			VALUES (gen_random_uuid(), v_enrollment_id, v_new_id, 'active', p_root_node_id);
			INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision)
			VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, 'enrolled');

			IF p_frequency = 'once' THEN
				INSERT INTO automation_trigger_log (id, automation_id, contact_email, customer_id, triggered_at)
				VALUES (gen_random_uuid()::text, p_automation_id, p_contact_email, v_customer_id, NOW())
				ON CONFLICT (automation_id, contact_email) DO UPDATE SET customer_id = COALESCE(automation_trigger_log.customer_id, EXCLUDED.customer_id);
			END IF;
			IF p_origin_event_id IS NOT NULL THEN
				INSERT INTO automation_match_audit (event_id, automation_id, engine, matched, decision_hash, contact_automation_id, reason)
				VALUES (p_origin_event_id, p_automation_id, 'legacy', TRUE, md5(concat_ws(':', p_automation_id, COALESCE(v_automation_version, 1)::text, p_root_node_id, p_frequency)), v_new_id, jsonb_build_object('decision', 'enrolled', 'customer_id', v_customer_id))
				ON CONFLICT (event_id, automation_id, engine) DO NOTHING;
			END IF;
			UPDATE automations SET stats = jsonb_set(COALESCE(stats, '{}'::jsonb), '{enrolled}', to_jsonb(COALESCE((stats->>'enrolled')::int, 0) + 1)), updated_at = NOW() WHERE id = p_automation_id;
			INSERT INTO automation_node_executions (id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, output)
			VALUES (gen_random_uuid()::text, v_new_id, p_automation_id, p_root_node_id, 'trigger', 'entered', NOW(), jsonb_build_object('customer_id', v_customer_id));
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (p_contact_email, 'insert', 'automation', 'automation.start', p_automation_id, jsonb_build_object('automation_id', jsonb_build_object('new', p_automation_id), 'root_node_id', jsonb_build_object('new', p_root_node_id), 'customer_id', jsonb_build_object('new', v_customer_id)), NOW());
		END;
		$$ LANGUAGE plpgsql`
}
