package repository

import (
	"context"
	"database/sql"
	"fmt"
)

func markBusinessProjectionDirty(ctx context.Context, tx *sql.Tx, kind, id string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id,next_check) VALUES($1,$2,NOW()) ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=NOW()`, kind, id)
	return err
}

// Source Customer rows retain external_user_id and redirect through merged_into_id;
// these durable rows are aliases rather than a second identity authority.
func moveBusinessCustomerAggregate(ctx context.Context, tx *sql.Tx, source, target string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE customer_external_id_aliases SET customer_id=$2 WHERE customer_id=$1`, source, target); err != nil {
		return err
	}
	statements := []string{`UPDATE business_objects SET customer_id=$2 WHERE customer_id=$1`, `UPDATE event_report_receipts SET customer_id=$2::uuid,response=jsonb_set(response,'{customer_id}',to_jsonb($2::text)) WHERE customer_id=$1`, `INSERT INTO customer_attribute_values(customer_id,attribute_key,value,definition_revision,updated_at) SELECT $2::uuid,attribute_key,value,definition_revision,updated_at FROM customer_attribute_values WHERE customer_id=$1 ON CONFLICT(customer_id,attribute_key) DO NOTHING`}
	for _, q := range statements {
		if _, err := tx.ExecContext(ctx, q, source, target); err != nil {
			return fmt.Errorf("merge business customer aggregate: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM customer_attribute_values WHERE customer_id=$1`, source); err != nil {
		return err
	}
	canonicalStatements := []string{
		`UPDATE event_ledger SET customer_id=$2::uuid,subject_id=CASE WHEN subject_type='customer' AND subject_id=$1::text THEN $2::text ELSE subject_id END WHERE customer_id=$1::uuid`,
		`UPDATE event_outbox SET payload=jsonb_set(jsonb_set(payload,'{subject,customer_id}',to_jsonb($2::text)),'{subject,id}',CASE WHEN payload->'subject'->>'type'='customer' AND payload->'subject'->>'id'=$1::text THEN to_jsonb($2::text) ELSE payload->'subject'->'id' END) WHERE payload->'subject'->>'customer_id'=$1::text`,
		`UPDATE journey_event_subjects SET customer_id=$2::uuid,subject_id=CASE WHEN subject_type='customer' AND subject_id=$1::text THEN $2::text ELSE subject_id END WHERE customer_id=$1::uuid`,
		`UPDATE configured_event_jobs SET customer_id=$2 WHERE customer_id=$1`,
		`UPDATE subject_attribute_history SET customer_id=$2 WHERE customer_id=$1`,
		`DELETE FROM marketing_trigger_subject_entries src USING marketing_trigger_subject_entries dst WHERE src.customer_id=$1::uuid AND src.subject_type='customer' AND dst.automation_id=src.automation_id AND dst.subject_type='customer' AND dst.subject_id=$2::text`,
		`UPDATE marketing_trigger_subject_entries SET customer_id=$2::uuid,subject_id=CASE WHEN subject_type='customer' AND subject_id=$1::text THEN $2::text ELSE subject_id END WHERE customer_id=$1::uuid`,
	}
	for _, q := range canonicalStatements {
		if _, err := tx.ExecContext(ctx, q, source, target); err != nil {
			return fmt.Errorf("canonicalize merged business history: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subject_state_history SET customer_id=$2 WHERE customer_id=$1`, source, target); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id,next_check) SELECT object_type,id::text,NOW() FROM business_objects WHERE customer_id=$1 ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=NOW()`, target); err != nil {
		return err
	}
	if err := markBusinessProjectionDirty(ctx, tx, "customer", target); err != nil {
		return err
	}
	return nil
}
