package schema

// JourneyTableDefinitions establishes Customer/Event as the authority for
// Automation enrollment while retaining the legacy Email projection.
func JourneyTableDefinitions() []string {
	return []string{
		`ALTER TABLE event_ledger ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE RESTRICT`,
		`CREATE INDEX IF NOT EXISTS idx_event_ledger_customer_occurred ON event_ledger(customer_id, occurred_at DESC) WHERE customer_id IS NOT NULL`,
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
		`CREATE OR REPLACE FUNCTION notifuse_project_journey_instance_state()
		RETURNS TRIGGER AS $$
		DECLARE v_updated INTEGER := 0;
		BEGIN
			UPDATE journey_instances
			SET status = NEW.status, current_node_id = NEW.current_node_id,
				next_scheduled_at = NEW.scheduled_at,
				completed_at = CASE WHEN NEW.status IN ('completed', 'exited', 'failed')
					THEN COALESCE(completed_at, CURRENT_TIMESTAMP) ELSE NULL END,
				updated_at = CURRENT_TIMESTAMP
			WHERE contact_automation_id = NEW.id;
			GET DIAGNOSTICS v_updated = ROW_COUNT;
			IF v_updated > 0 THEN
				INSERT INTO journey_instance_events (id, journey_instance_id, node_id, event_type, status, reason, payload)
				SELECT gen_random_uuid(), instance.id, NEW.current_node_id, 'state_changed', NEW.status,
					NEW.exit_reason, jsonb_build_object('scheduled_at', NEW.scheduled_at)
				FROM journey_instances instance WHERE instance.contact_automation_id = NEW.id;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS contact_automation_journey_projection ON contact_automations`,
		`CREATE TRIGGER contact_automation_journey_projection
		AFTER UPDATE OF current_node_id, status, scheduled_at, exit_reason ON contact_automations
		FOR EACH ROW WHEN (
			OLD.current_node_id IS DISTINCT FROM NEW.current_node_id OR
			OLD.status IS DISTINCT FROM NEW.status OR
			OLD.scheduled_at IS DISTINCT FROM NEW.scheduled_at OR
			OLD.exit_reason IS DISTINCT FROM NEW.exit_reason
		) EXECUTE FUNCTION notifuse_project_journey_instance_state()`,
	}
}

// JourneyAutomationEnrollContactFunction installs the Customer/Event authority
// and the legacy Email adapter in one statement. New callers use the typed
// automation_enroll_customer result; already-installed triggers keep calling
// automation_enroll_contact and resolve their Email projection first.
func JourneyAutomationEnrollContactFunction() string {
	return `CREATE OR REPLACE FUNCTION automation_enroll_customer(
			p_automation_id VARCHAR(36),
			p_customer_id UUID,
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_frequency VARCHAR(20),
			p_origin_event_id UUID DEFAULT NULL,
			p_entry_guard JSONB DEFAULT '{"enabled":false}'::jsonb,
			p_expected_version INTEGER DEFAULT NULL,
			p_engine TEXT DEFAULT 'legacy'
		) RETURNS TABLE(outcome TEXT, contact_automation_id VARCHAR(36), retry_at TIMESTAMPTZ) AS $$
		DECLARE
			v_customer_id UUID;
			v_enrollment_id UUID;
			v_new_id VARCHAR(36);
			v_instance_id UUID;
			v_automation_version INTEGER;
			v_dedupe_key TEXT;
			v_inserted INTEGER := 0;
			v_existing_retry_at TIMESTAMPTZ;
			v_latest_started_at TIMESTAMPTZ;
			v_active_count INTEGER := 0;
			v_max_concurrent INTEGER := 0;
			v_cooldown_ns BIGINT := 0;
			v_cooldown INTERVAL := INTERVAL '0 seconds';
			v_guard_enabled BOOLEAN := FALSE;
		BEGIN
			outcome := NULL; contact_automation_id := NULL; retry_at := NULL;
			SELECT COALESCE(customer.merged_into_id, customer.id) INTO v_customer_id
			FROM customers customer WHERE customer.id = p_customer_id;
			IF v_customer_id IS NULL THEN
				outcome := 'guard_denied'; RETURN NEXT; RETURN;
			END IF;

			SELECT automation.version INTO v_automation_version
			FROM automations automation
			WHERE automation.id = p_automation_id AND automation.status = 'live'
				AND automation.deleted_at IS NULL
				AND (p_expected_version IS NULL OR automation.version = p_expected_version);
			IF v_automation_version IS NULL THEN
				INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason)
				VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, 'guard_denied', 'automation_not_live_or_version_changed');
				outcome := 'guard_denied'; RETURN NEXT; RETURN;
			END IF;

			IF NULLIF(BTRIM(p_contact_email), '') IS NULL THEN
				SELECT contact.email INTO p_contact_email FROM contacts contact
				WHERE contact.customer_id = v_customer_id ORDER BY contact.updated_at DESC LIMIT 1;
			END IF;
			IF NULLIF(BTRIM(p_contact_email), '') IS NULL THEN
				INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason)
				VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, 'guard_denied', 'contact_email_projection_missing');
				outcome := 'guard_denied'; RETURN NEXT; RETURN;
			END IF;

			IF p_frequency = 'once' THEN
				v_dedupe_key := encode(sha256(convert_to(concat_ws(':', p_automation_id, v_customer_id::text, 'once'), 'UTF8')), 'hex');
				INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, dedupe_key, entry_guard)
				VALUES (gen_random_uuid(), p_automation_id, v_automation_version, v_customer_id, p_contact_email, 'once', v_dedupe_key, COALESCE(p_entry_guard, '{"enabled":false}'::jsonb))
				ON CONFLICT DO NOTHING RETURNING id INTO v_enrollment_id;
			ELSIF p_frequency = 'every_time' AND p_origin_event_id IS NOT NULL THEN
				v_dedupe_key := encode(sha256(convert_to(concat_ws(':', p_automation_id, v_customer_id::text, 'every_time', p_origin_event_id::text), 'UTF8')), 'hex');
				INSERT INTO journey_enrollments (id, automation_id, automation_version, customer_id, contact_email, frequency, origin_event_id, dedupe_key, entry_guard)
				VALUES (gen_random_uuid(), p_automation_id, v_automation_version, v_customer_id, p_contact_email, 'every_time', p_origin_event_id, v_dedupe_key, COALESCE(p_entry_guard, '{"enabled":false}'::jsonb))
				ON CONFLICT DO NOTHING RETURNING id INTO v_enrollment_id;
			ELSE
				INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason)
				VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, 'guard_denied', 'invalid_frequency_or_missing_event_id');
				outcome := 'guard_denied'; RETURN NEXT; RETURN;
			END IF;
			GET DIAGNOSTICS v_inserted = ROW_COUNT;

			IF v_inserted = 0 THEN
				SELECT enrollment.id INTO v_enrollment_id FROM journey_enrollments enrollment
				WHERE enrollment.automation_id = p_automation_id AND enrollment.customer_id = v_customer_id
					AND ((p_frequency = 'once' AND enrollment.frequency = 'once')
						OR (p_frequency = 'every_time' AND enrollment.frequency = 'every_time' AND enrollment.origin_event_id = p_origin_event_id))
				FOR UPDATE;
				IF EXISTS (SELECT 1 FROM journey_instances instance WHERE instance.enrollment_id = v_enrollment_id) THEN
					outcome := CASE WHEN p_frequency = 'once' THEN 'already_once' ELSE 'replayed_event' END;
					INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason)
					VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, outcome, 'database_unique_constraint');
					RETURN NEXT; RETURN;
				END IF;
				SELECT decision.retry_at INTO v_existing_retry_at FROM journey_entry_decisions decision
				WHERE decision.automation_id = p_automation_id AND decision.customer_id = v_customer_id
					AND decision.decision = 'guard_deferred'
					AND decision.origin_event_id IS NOT DISTINCT FROM p_origin_event_id
				ORDER BY decision.decided_at DESC LIMIT 1;
				IF v_existing_retry_at IS NULL OR v_existing_retry_at > CURRENT_TIMESTAMP THEN
					outcome := CASE WHEN v_existing_retry_at IS NOT NULL THEN 'guard_deferred'
						WHEN p_frequency = 'once' THEN 'already_once' ELSE 'replayed_event' END;
					retry_at := v_existing_retry_at;
					RETURN NEXT; RETURN;
				END IF;
			END IF;

			v_guard_enabled := COALESCE((p_entry_guard->>'enabled')::BOOLEAN, FALSE);
			IF v_guard_enabled THEN
				v_cooldown_ns := GREATEST(COALESCE((p_entry_guard->>'cooldown')::BIGINT, 0), 0);
				v_max_concurrent := GREATEST(COALESCE((p_entry_guard->>'max_concurrent')::INTEGER, 0), 0);
				v_cooldown := (v_cooldown_ns / 1000000000.0) * INTERVAL '1 second';
				SELECT COUNT(*) FILTER (WHERE instance.status = 'active'), MAX(instance.started_at)
				INTO v_active_count, v_latest_started_at
				FROM journey_instances instance JOIN journey_enrollments enrollment ON enrollment.id = instance.enrollment_id
				WHERE enrollment.automation_id = p_automation_id AND enrollment.customer_id = v_customer_id;
				IF v_max_concurrent > 0 AND v_active_count >= v_max_concurrent THEN
					IF v_cooldown_ns > 0 THEN
						retry_at := GREATEST(CURRENT_TIMESTAMP + INTERVAL '1 minute', COALESCE(v_latest_started_at, CURRENT_TIMESTAMP) + v_cooldown);
						outcome := 'guard_deferred';
					ELSE
						outcome := 'guard_denied';
					END IF;
					INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason, retry_at)
					VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, outcome, 'max_concurrent_reached', retry_at);
					RETURN NEXT; RETURN;
				END IF;
				IF v_cooldown_ns > 0 AND v_latest_started_at IS NOT NULL AND v_latest_started_at + v_cooldown > CURRENT_TIMESTAMP THEN
					retry_at := v_latest_started_at + v_cooldown;
					outcome := 'guard_deferred';
					INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision, reason, retry_at)
					VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, outcome, 'cooldown_active', retry_at);
					RETURN NEXT; RETURN;
				END IF;
			END IF;

			v_new_id := gen_random_uuid()::text;
			v_instance_id := gen_random_uuid();
			INSERT INTO contact_automations (id, automation_id, contact_email, customer_id, current_node_id, status, entered_at, scheduled_at, origin_event_id, automation_version)
			VALUES (v_new_id, p_automation_id, p_contact_email, v_customer_id, p_root_node_id, 'active', NOW(), NOW(), p_origin_event_id, v_automation_version);
			INSERT INTO journey_instances (id, enrollment_id, contact_automation_id, status, current_node_id)
			VALUES (v_instance_id, v_enrollment_id, v_new_id, 'active', p_root_node_id);
			INSERT INTO journey_instance_events (id, journey_instance_id, node_id, event_type, status, payload)
			VALUES (gen_random_uuid(), v_instance_id, p_root_node_id, 'enrolled', 'active', jsonb_build_object('customer_id', v_customer_id, 'origin_event_id', p_origin_event_id));
			INSERT INTO journey_entry_decisions (id, automation_id, customer_id, origin_event_id, decision)
			VALUES (gen_random_uuid(), p_automation_id, v_customer_id, p_origin_event_id, 'enrolled');

			IF p_frequency = 'once' THEN
				INSERT INTO automation_trigger_log (id, automation_id, contact_email, customer_id, triggered_at)
				VALUES (gen_random_uuid()::text, p_automation_id, p_contact_email, v_customer_id, NOW())
				ON CONFLICT (automation_id, contact_email) DO UPDATE SET customer_id = COALESCE(automation_trigger_log.customer_id, EXCLUDED.customer_id);
			END IF;
			IF p_origin_event_id IS NOT NULL AND p_engine = 'legacy' THEN
				INSERT INTO automation_match_audit (event_id, automation_id, engine, matched, decision_hash, contact_automation_id, reason)
				VALUES (p_origin_event_id, p_automation_id, 'legacy', TRUE, md5(concat_ws(':', p_automation_id, v_automation_version::text, p_root_node_id, p_frequency)), v_new_id, jsonb_build_object('decision', 'enrolled', 'customer_id', v_customer_id))
				ON CONFLICT (event_id, automation_id, engine) DO NOTHING;
			END IF;
			UPDATE automations SET stats = jsonb_set(COALESCE(stats, '{}'::jsonb), '{enrolled}', to_jsonb(COALESCE((stats->>'enrolled')::int, 0) + 1)), updated_at = NOW()
			WHERE id = p_automation_id AND version = v_automation_version;
			INSERT INTO automation_node_executions (id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, output)
			VALUES (gen_random_uuid()::text, v_new_id, p_automation_id, p_root_node_id, 'trigger', 'entered', NOW(), jsonb_build_object('customer_id', v_customer_id));
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at, customer_id)
			VALUES (p_contact_email, 'insert', 'automation', 'automation.start', p_automation_id,
				jsonb_build_object('automation_id', jsonb_build_object('new', p_automation_id), 'root_node_id', jsonb_build_object('new', p_root_node_id), 'customer_id', jsonb_build_object('new', v_customer_id)), NOW(), v_customer_id);
			outcome := 'enrolled'; contact_automation_id := v_new_id; RETURN NEXT; RETURN;
		END;
		$$ LANGUAGE plpgsql;

		CREATE OR REPLACE FUNCTION automation_enroll_contact(
			p_automation_id VARCHAR(36),
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_frequency VARCHAR(20),
			p_origin_event_id UUID DEFAULT NULL
		) RETURNS VOID AS $$
		DECLARE
			v_customer_id UUID;
		BEGIN
			SELECT customer_id INTO v_customer_id FROM contacts WHERE LOWER(BTRIM(email)) = LOWER(BTRIM(p_contact_email));
			IF v_customer_id IS NULL THEN
				INSERT INTO journey_identity_reconciliation (automation_id, contact_email, origin_event_id, reason)
				VALUES (p_automation_id, p_contact_email, p_origin_event_id, 'customer_not_resolved')
				ON CONFLICT (automation_id, contact_email) DO UPDATE SET last_seen_at = CURRENT_TIMESTAMP,
					occurrences = journey_identity_reconciliation.occurrences + 1, origin_event_id = EXCLUDED.origin_event_id;
				RETURN;
			END IF;
			PERFORM outcome FROM automation_enroll_customer(
				p_automation_id, v_customer_id, p_contact_email, p_root_node_id,
				p_frequency, p_origin_event_id, '{"enabled":false}'::jsonb, NULL, 'legacy'
			);
		END;
		$$ LANGUAGE plpgsql`
}
