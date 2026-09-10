package repository

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *BusinessConfigPostgresRepository) ExtendEvents(ctx context.Context, workspace string, keys []string, extension map[string]any) ([]domain.BusinessDefinition, error) {
	db, err := r.getDB(ctx, workspace)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('business_definitions',0))`); err != nil {
		return nil, err
	}
	result := make([]domain.BusinessDefinition, 0, len(keys))
	for _, key := range keys {
		definition, err := scanBusinessDefinition(tx.QueryRowContext(ctx, `SELECT `+businessDefinitionColumns+` FROM business_definitions WHERE kind='event' AND key=$1 FOR UPDATE`, key))
		if err != nil {
			return nil, err
		}
		if definition.Status == "archived" {
			return nil, domain.NewValidationError("archived events cannot receive new report fields")
		}
		merged, err := domain.ExtendEventSchema(definition.Schema, extension)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}
		saved, err := scanBusinessDefinition(tx.QueryRowContext(ctx, `UPDATE business_definitions SET schema=$2,revision=revision+1,updated_at=NOW() WHERE id=$1 RETURNING `+businessDefinitionColumns, definition.ID, raw))
		if err != nil {
			return nil, err
		}
		document, err := json.Marshal(saved)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO business_definition_revisions(definition_id,revision,document) VALUES($1,$2,$3)`, saved.ID, saved.Revision, document); err != nil {
			return nil, err
		}
		result = append(result, *saved)
	}
	return result, tx.Commit()
}
