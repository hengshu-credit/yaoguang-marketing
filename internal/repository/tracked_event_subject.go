package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func resolveTrackedSubject(ctx context.Context, tx *sql.Tx, c domain.TrackedEventCommand, customerID string, eventID uuid.UUID) (domain.EventSubject, error) {
	var err error
	subject := domain.EventSubject{Type: "customer", ID: customerID, CustomerID: customerID}
	if customerID == "" {
		subject.Type = "unresolved"
		subject.ID = eventID.String()
	}
	if objectType, _ := c.Definition.Settings["subject_type"].(string); objectType != "" && objectType != "customer" && customerID != "" {
		if c.ObjectDefinition == nil {
			return domain.EventSubject{}, domain.NewValidationError("original object definition snapshot is missing")
		}
		path, _ := c.Definition.Settings["subject_id_path"].(string)
		raw := map[string]any{"data": c.Event.Data, "customer_attributes": c.Event.CustomerAttributes}
		value, exists := domain.BusinessJSONPath(raw, path)
		key, ok := value.(string)
		if !exists || !ok || key == "" {
			return domain.EventSubject{}, domain.NewValidationError("subject_id_path must resolve to a nonempty string")
		}
		if trackedEventIsHistorical(c.Event) {
			var id, owner string
			err := tx.QueryRowContext(ctx, `SELECT id::text,customer_id::text FROM business_objects WHERE object_type=$1 AND source=$2 AND external_key=$3`, objectType, c.Source, key).Scan(&id, &owner)
			if errors.Is(err, sql.ErrNoRows) {
				// Preserve object identity for historical analysis without applying
				// historical values to the object's current attribute projection.
				if _, err = tx.ExecContext(ctx, `INSERT INTO business_objects(id,object_type,source,external_key,customer_id,attributes,definition_revision,created_at) VALUES($1,$2,$3,$4,$5,'{}',$6,$7) ON CONFLICT(object_type,source,external_key) DO NOTHING`, uuid.New(), objectType, c.Source, key, customerID, c.ObjectDefinition.Revision, c.Event.TriggeredAt); err != nil {
					return domain.EventSubject{}, err
				}
				err = tx.QueryRowContext(ctx, `SELECT id::text,customer_id::text FROM business_objects WHERE object_type=$1 AND source=$2 AND external_key=$3`, objectType, c.Source, key).Scan(&id, &owner)
			}
			if err != nil {
				return domain.EventSubject{}, err
			}
			if owner != customerID {
				return domain.EventSubject{}, domain.NewValidationError("business object belongs to a different customer")
			}
			subject.Type = objectType
			subject.ID = id
			return subject, nil
		}
		revision := c.ObjectDefinition.Revision
		objectSchema := c.ObjectDefinition.Schema
		objectData := map[string]any{}
		if p, ok := c.Definition.Settings["subject_attributes_path"].(string); ok {
			v, exists := domain.BusinessJSONPath(raw, p)
			if !exists {
				return domain.EventSubject{}, domain.NewValidationError("subject_attributes_path is missing")
			}
			objectData, ok = v.(map[string]any)
			if !ok {
				return domain.EventSubject{}, domain.NewValidationError("subject attributes must be an object")
			}
			if err = domain.ValidateBusinessValue(objectSchema, objectData, "subject"); err != nil {
				return domain.EventSubject{}, domain.NewValidationError(err.Error())
			}
		}
		attrs, _ := json.Marshal(objectData)
		var objectID string
		err = tx.QueryRowContext(ctx, `INSERT INTO business_objects(id,object_type,source,external_key,customer_id,attributes,definition_revision) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(object_type,source,external_key) DO UPDATE SET attributes=business_objects.attributes||EXCLUDED.attributes,updated_at=NOW() WHERE business_objects.customer_id=EXCLUDED.customer_id RETURNING id`, uuid.New(), objectType, c.Source, key, customerID, attrs, revision).Scan(&objectID)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EventSubject{}, domain.NewValidationError("business object belongs to a different customer")
		}
		if err != nil {
			return domain.EventSubject{}, err
		}
		subject.Type = objectType
		subject.ID = objectID
	}
	return subject, nil
}
