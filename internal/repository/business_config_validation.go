package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func validateBusinessPublicationTx(ctx context.Context, tx *sql.Tx, d *domain.BusinessDefinition) error {
	if d.Status != "active" {
		return nil
	}
	if d.Kind == "object_type" {
		rows, err := tx.QueryContext(ctx, `SELECT attributes FROM business_objects WHERE object_type=$1`, d.Key)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				return err
			}
			var attributes map[string]any
			if err = json.Unmarshal(raw, &attributes); err != nil {
				return err
			}
			if err = domain.ValidateBusinessValue(d.Schema, attributes, "existing_object"); err != nil {
				return domain.NewValidationError("schema is incompatible with existing objects: " + err.Error())
			}
		}
		return rows.Err()
	}
	if d.Kind != "object_relation" {
		return nil
	}
	from, _ := d.Settings["from_type"].(string)
	to, _ := d.Settings["to_type"].(string)
	var cycle bool
	if err := tx.QueryRowContext(ctx, `WITH RECURSIVE reachable(node) AS (SELECT $1::text UNION SELECT d.settings->>'to_type' FROM business_definitions d JOIN reachable r ON d.settings->>'from_type'=r.node WHERE d.kind='object_relation' AND d.status='active' AND d.key<>$3) SELECT EXISTS(SELECT 1 FROM reachable WHERE node=$2)`, to, from, d.Key).Scan(&cycle); err != nil {
		return err
	}
	if cycle {
		return domain.NewValidationError("object relation dependency cycle")
	}
	card, _ := d.Settings["cardinality"].(string)
	if card == "one_to_many" || card == "one_to_one" {
		var conflict bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT to_id FROM business_object_relations WHERE relation_key=$1 AND valid_to IS NULL GROUP BY to_id HAVING count(DISTINCT from_id)>1) OR ($2='one_to_one' AND EXISTS(SELECT from_id FROM business_object_relations WHERE relation_key=$1 AND valid_to IS NULL GROUP BY from_id HAVING count(DISTINCT to_id)>1))`, d.Key, card).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return domain.NewValidationError(fmt.Sprintf("existing instances conflict with %s cardinality", card))
		}
	}
	return nil
}
