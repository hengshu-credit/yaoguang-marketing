package schema

import (
	"fmt"
	"time"
)

const EventLedgerTableName = "event_ledger"

// RealtimeTableDefinitions returns the workspace-local source-of-truth schema
// for the durable realtime runtime. The event ledger is partitioned, while the
// separate idempotency table provides event-ID uniqueness across all months.
func RealtimeTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS event_idempotency (
			id UUID PRIMARY KEY,
			received_at TIMESTAMPTZ NOT NULL,
			payload_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS event_ledger (
			id UUID NOT NULL,
			event_type TEXT NOT NULL,
			subject_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			contact_email TEXT,
			source TEXT NOT NULL,
			schema_version INTEGER NOT NULL DEFAULT 1,
			occurred_at TIMESTAMPTZ NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			sequence BIGINT,
			properties JSONB NOT NULL DEFAULT '{}'::jsonb,
			context JSONB NOT NULL DEFAULT '{}'::jsonb,
			timeline_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (received_at, id)
		) PARTITION BY RANGE (received_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_ledger_type_received
			ON event_ledger (event_type, received_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_event_ledger_subject_occurred
			ON event_ledger (subject_type, subject_id, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_event_ledger_id
			ON event_ledger (id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_ledger_timeline
			ON event_ledger (timeline_id) WHERE timeline_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS event_outbox (
			id UUID PRIMARY KEY,
			event_id UUID NOT NULL,
			topic TEXT NOT NULL,
			routing_key TEXT NOT NULL,
			payload JSONB NOT NULL,
			headers JSONB NOT NULL DEFAULT '{}'::jsonb,
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'claimed', 'published', 'dead')),
			attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			claimed_by TEXT,
			claim_token UUID,
			claim_expires_at TIMESTAMPTZ,
			published_at TIMESTAMPTZ,
			last_error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (event_id, topic, routing_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_outbox_available
			ON event_outbox (available_at, created_at)
			WHERE status IN ('pending', 'claimed')`,
		`CREATE TABLE IF NOT EXISTS consumer_inbox (
			consumer TEXT NOT NULL,
			message_id UUID NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('processing', 'completed', 'failed')),
			attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			claim_token UUID NOT NULL,
			claim_expires_at TIMESTAMPTZ NOT NULL,
			processed_at TIMESTAMPTZ,
			last_error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (consumer, message_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_consumer_inbox_reclaim
			ON consumer_inbox (consumer, claim_expires_at)
			WHERE status = 'processing'`,
		`CREATE TABLE IF NOT EXISTS automation_trigger_bindings (
			automation_id TEXT NOT NULL,
			automation_version INTEGER NOT NULL CHECK (automation_version > 0),
			event_type TEXT NOT NULL,
			subject_type TEXT NOT NULL,
			dependency_keys TEXT[] NOT NULL DEFAULT '{}'::text[],
			condition_hash TEXT NOT NULL,
			compiled_condition JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (automation_id, automation_version, event_type, subject_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_trigger_bindings_candidates
			ON automation_trigger_bindings (event_type, subject_type, automation_version)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_trigger_bindings_dependencies
			ON automation_trigger_bindings USING GIN (dependency_keys)`,
		`CREATE TABLE IF NOT EXISTS automation_match_audit (
			event_id UUID NOT NULL,
			automation_id TEXT NOT NULL,
			engine TEXT NOT NULL CHECK (engine IN ('legacy', 'realtime')),
			matched BOOLEAN NOT NULL,
			decision_hash TEXT NOT NULL,
			contact_automation_id TEXT,
			reason JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (event_id, automation_id, engine)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_match_audit_compare
			ON automation_match_audit (created_at, event_id, automation_id)`,
		`CREATE TABLE IF NOT EXISTS side_effect_executions (
			effect_key TEXT PRIMARY KEY,
			contact_automation_id TEXT NOT NULL,
			automation_version INTEGER NOT NULL CHECK (automation_version > 0),
			node_id TEXT NOT NULL,
			execution_version BIGINT NOT NULL CHECK (execution_version >= 0),
			channel TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('reserved', 'submitted', 'confirmed', 'failed', 'unknown')),
			provider_message_id TEXT,
			request_hash TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			last_error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE automations
			ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE contact_timeline
			ADD COLUMN IF NOT EXISTS origin_event_id UUID`,
		`UPDATE contact_timeline
			SET origin_event_id = gen_random_uuid()
			WHERE origin_event_id IS NULL`,
		`ALTER TABLE contact_timeline
			ALTER COLUMN origin_event_id SET DEFAULT gen_random_uuid(),
			ALTER COLUMN origin_event_id SET NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_timeline_origin_event
			ON contact_timeline (origin_event_id)`,
		`ALTER TABLE contact_automations
			ADD COLUMN IF NOT EXISTS origin_event_id UUID,
			ADD COLUMN IF NOT EXISTS automation_version INTEGER NOT NULL DEFAULT 1,
			ADD COLUMN IF NOT EXISTS state_version BIGINT NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS claim_token UUID,
			ADD COLUMN IF NOT EXISTS claimed_by TEXT,
			ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS claim_expires_at TIMESTAMPTZ`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_automations_origin_event
			ON contact_automations (automation_id, origin_event_id)
			WHERE origin_event_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_contact_automations_claimable
			ON contact_automations (scheduled_at, claim_expires_at)
			WHERE status = 'active'`,
		`DROP FUNCTION IF EXISTS automation_enroll_contact(VARCHAR(36), VARCHAR(255), VARCHAR(36), VARCHAR(20))`,
		AutomationEnrollContactFunction(),
		TimelineEventBridgeFunction(),
		`DROP TRIGGER IF EXISTS contact_timeline_realtime_bridge ON contact_timeline`,
		`CREATE TRIGGER contact_timeline_realtime_bridge
			AFTER INSERT ON contact_timeline
			FOR EACH ROW EXECUTE FUNCTION notifuse_capture_timeline_event()`,
	}
}

// EventLedgerPartitionDDL creates one UTC calendar-month partition. Names and
// bounds are derived only from time, so the result is safe to execute directly.
func EventLedgerPartitionDDL(month time.Time) string {
	start := time.Date(month.UTC().Year(), month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS event_ledger_%s PARTITION OF event_ledger FOR VALUES FROM ('%s') TO ('%s')",
		start.Format("200601"),
		start.Format("2006-01-02T15:04:05Z07:00"),
		end.Format("2006-01-02T15:04:05Z07:00"),
	)
}

func RealtimeBootstrapMonths(now time.Time) []time.Time {
	return []time.Time{
		now.AddDate(0, -1, 0),
		now,
		now.AddDate(0, 1, 0),
		now.AddDate(0, 2, 0),
	}
}

// TimelineEventBridgeFunction captures one immutable event and one outbox row
// for each timeline insert. It never reads automation definitions, so its cost
// does not grow with the number of live campaigns.
func TimelineEventBridgeFunction() string {
	return `CREATE OR REPLACE FUNCTION notifuse_capture_timeline_event()
	RETURNS TRIGGER AS $$
	DECLARE
		v_registered UUID;
		v_message_id UUID := gen_random_uuid();
		v_received_at TIMESTAMPTZ := COALESCE(NEW.db_created_at, clock_timestamp());
		v_data JSONB := jsonb_build_object(
			'operation', NEW.operation,
			'entity_type', NEW.entity_type,
			'entity_id', NEW.entity_id,
			'changes', COALESCE(NEW.changes, '{}'::jsonb)
		);
	BEGIN
		INSERT INTO event_idempotency (id, received_at, payload_hash)
		VALUES (NEW.origin_event_id, v_received_at, md5(v_data::text))
		ON CONFLICT (id) DO NOTHING
		RETURNING id INTO v_registered;

		IF v_registered IS NULL THEN
			UPDATE event_ledger
			SET timeline_id = COALESCE(timeline_id, NEW.id)
			WHERE id = NEW.origin_event_id;
			RETURN NEW;
		END IF;

		INSERT INTO event_ledger (
			id, event_type, subject_type, subject_id, customer_id, contact_email, source,
			schema_version, occurred_at, received_at, properties, context, timeline_id
		) VALUES (
			NEW.origin_event_id,
			NEW.kind,
			NEW.entity_type,
			COALESCE(NEW.entity_id, NEW.email),
			NEW.customer_id,
			NEW.email,
			'contact_timeline',
			1,
			NEW.created_at,
			v_received_at,
			COALESCE(NEW.changes, '{}'::jsonb),
			jsonb_build_object('operation', NEW.operation),
			NEW.id
		);

		INSERT INTO event_outbox (
			id, event_id, topic, routing_key, payload, headers
		) VALUES (
			v_message_id,
			NEW.origin_event_id,
			'notifuse.events',
			NEW.kind,
			jsonb_build_object(
				'id', v_message_id,
				'event_id', NEW.origin_event_id,
				'type', NEW.kind,
				'schema_version', 1,
				'workspace_id', current_database(),
				'subject', jsonb_build_object(
					'type', NEW.entity_type,
					'id', COALESCE(NEW.entity_id, NEW.email),
					'customer_id', NEW.customer_id,
					'contact_email', NEW.email
				),
				'source', 'contact_timeline',
				'occurred_at', NEW.created_at,
				'received_at', v_received_at,
				'correlation_id', NEW.origin_event_id,
				'data', v_data
			),
			jsonb_build_object('schema_version', 1)
		)
		ON CONFLICT (event_id, topic, routing_key) DO NOTHING;

		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`
}
