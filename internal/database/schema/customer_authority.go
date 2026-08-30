package schema

import "fmt"

// CustomerAuthorityTableDefinitions adds UUID Customer references to the
// legacy marketing tables without removing their email compatibility keys.
// The same statements are used by fresh workspace initialization and V47.
func CustomerAuthorityTableDefinitions() []string {
	type customerReference struct {
		table string
		index string
	}

	references := []customerReference{
		{table: "contact_lists", index: "customer_id, list_id, status"},
		{table: "contact_segments", index: "customer_id, segment_id, version"},
		{table: "custom_events", index: "customer_id, occurred_at DESC"},
		{table: "contact_timeline", index: "customer_id, created_at DESC, id DESC"},
		{table: "contact_automations", index: "customer_id, automation_id, status"},
		{table: "automation_trigger_log", index: "customer_id, automation_id, triggered_at DESC"},
		{table: "message_history", index: "customer_id, sent_at DESC, id DESC"},
		{table: "email_queue", index: "customer_id, status, priority, created_at"},
	}

	statements := make([]string, 0, len(references)*3+5)
	for _, reference := range references {
		statements = append(statements,
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS customer_id UUID", reference.table),
			fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS idx_%s_customer_id ON %s (%s) WHERE customer_id IS NOT NULL",
				reference.table, reference.table, reference.index,
			),
			fmt.Sprintf(`DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint
					WHERE conname = '%[1]s_customer_id_fkey'
					AND conrelid = '%[1]s'::regclass
				) THEN
					ALTER TABLE %[1]s ADD CONSTRAINT %[1]s_customer_id_fkey
						FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
				END IF;
			END $$`, reference.table),
		)
	}

	return append(statements,
		`CREATE OR REPLACE FUNCTION populate_contact_timeline_customer_id()
		RETURNS TRIGGER AS $$
		BEGIN
			IF NEW.customer_id IS NULL AND NULLIF(BTRIM(NEW.email), '') IS NOT NULL THEN
				SELECT customer_id INTO NEW.customer_id FROM contacts WHERE email = NEW.email;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS contact_timeline_customer_authority_trigger ON contact_timeline`,
		`CREATE TRIGGER contact_timeline_customer_authority_trigger
			BEFORE INSERT OR UPDATE OF email ON contact_timeline
			FOR EACH ROW EXECUTE FUNCTION populate_contact_timeline_customer_id()`,
		`CREATE TABLE IF NOT EXISTS customer_projection_reconciliation (
			entity_name VARCHAR(64) PRIMARY KEY,
			missing_customer_id_count BIGINT NOT NULL DEFAULT 0 CHECK (missing_customer_id_count >= 0),
			conflict_count BIGINT NOT NULL DEFAULT 0 CHECK (conflict_count >= 0),
			last_scanned_at TIMESTAMPTZ,
			last_repaired_at TIMESTAMPTZ,
			last_error TEXT,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_projection_reconciliation_attention
			ON customer_projection_reconciliation (updated_at DESC, entity_name)
			WHERE missing_customer_id_count > 0 OR conflict_count > 0 OR last_error IS NOT NULL`,
	)
}
