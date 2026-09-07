package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type BusinessObjectPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewBusinessObjectRepository(w domain.WorkspaceRepository) *BusinessObjectPostgresRepository {
	return &BusinessObjectPostgresRepository{workspaceRepo: w}
}
func NewBusinessObjectRepositoryWithDB(db *sql.DB) *BusinessObjectPostgresRepository {
	return &BusinessObjectPostgresRepository{db: db}
}
func (r *BusinessObjectPostgresRepository) getDB(ctx context.Context, w string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	return r.workspaceRepo.GetConnection(ctx, w)
}

const businessObjectColumns = `id,object_type,source,external_key,customer_id,attributes,definition_revision,created_at,updated_at`

func scanBusinessObject(row businessScanner) (*domain.BusinessObject, error) {
	var o domain.BusinessObject
	var attrs []byte
	err := row.Scan(&o.ID, &o.ObjectType, &o.Source, &o.ExternalKey, &o.CustomerID, &attrs, &o.DefinitionRevision, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(attrs, &o.Attributes)
	return &o, err
}
func (r *BusinessObjectPostgresRepository) Upsert(ctx context.Context, w string, o *domain.BusinessObject) (*domain.BusinessObject, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	attrs, err := json.Marshal(o.Attributes)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	customerID := o.CustomerID
	for depth := 0; ; depth++ {
		if depth > 32 {
			return nil, domain.NewValidationError("customer merge chain is invalid")
		}
		var merged sql.NullString
		if err = tx.QueryRowContext(ctx, `SELECT merged_into_id FROM customers WHERE id=$1 FOR UPDATE`, customerID).Scan(&merged); err != nil {
			return nil, err
		}
		if !merged.Valid {
			break
		}
		customerID = merged.String
	}
	saved, err := scanBusinessObject(tx.QueryRowContext(ctx, `INSERT INTO business_objects(id,object_type,source,external_key,customer_id,attributes,definition_revision) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(object_type,source,external_key) DO UPDATE SET attributes=EXCLUDED.attributes,definition_revision=EXCLUDED.definition_revision,updated_at=NOW() WHERE business_objects.customer_id=EXCLUDED.customer_id RETURNING `+businessObjectColumns, uuid.New(), o.ObjectType, o.Source, o.ExternalKey, customerID, attrs, o.DefinitionRevision))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrBusinessDefinitionConflict
	}
	if err != nil {
		return nil, err
	}
	if err = markBusinessProjectionDirty(ctx, tx, saved.ObjectType, saved.ID); err != nil {
		return nil, err
	}
	if err = markBusinessProjectionDirty(ctx, tx, "customer", saved.CustomerID); err != nil {
		return nil, err
	}
	return saved, tx.Commit()
}
func (r *BusinessObjectPostgresRepository) Get(ctx context.Context, w, typ, source, key string) (*domain.BusinessObject, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	return scanBusinessObject(db.QueryRowContext(ctx, `SELECT `+businessObjectColumns+` FROM business_objects WHERE object_type=$1 AND source=$2 AND external_key=$3`, typ, source, key))
}
func (r *BusinessObjectPostgresRepository) List(ctx context.Context, w, customer, typ string, limit int) ([]domain.BusinessObject, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `SELECT `+businessObjectColumns+` FROM business_objects WHERE ($1='' OR customer_id=NULLIF($1,'')::uuid) AND ($2='' OR object_type=$2) ORDER BY created_at DESC LIMIT $3`, customer, typ, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BusinessObject{}
	for rows.Next() {
		o, err := scanBusinessObject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}
func (r *BusinessObjectPostgresRepository) Relate(ctx context.Context, w string, d *domain.BusinessDefinition, fromID, toID string) error {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, d.Key); err != nil {
		return err
	}
	var fromType, toType, fromCustomer, toCustomer, fromSource, toSource string
	if err = tx.QueryRowContext(ctx, `SELECT a.object_type,b.object_type,a.customer_id,b.customer_id,a.source,b.source FROM business_objects a,business_objects b WHERE a.id=$1 AND b.id=$2 FOR UPDATE`, fromID, toID).Scan(&fromType, &toType, &fromCustomer, &toCustomer, &fromSource, &toSource); err != nil {
		return err
	}
	if fromType != d.Settings["from_type"] || toType != d.Settings["to_type"] || fromCustomer != toCustomer || fromSource != toSource {
		return domain.NewValidationError("relation types, customer ownership and source must match")
	}
	card, _ := d.Settings["cardinality"].(string)
	var conflict bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM business_object_relations WHERE relation_key=$1 AND valid_to IS NULL AND ((($4='one_to_one' OR $4='one_to_many') AND to_id=$3 AND from_id<>$2) OR ($4='one_to_one' AND from_id=$2 AND to_id<>$3)))`, d.Key, fromID, toID, card).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return domain.NewValidationError("relation cardinality conflict")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO business_object_relations(id,relation_key,from_id,to_id) VALUES($1,$2,$3,$4) ON CONFLICT(relation_key,from_id,to_id) DO NOTHING`, uuid.New(), d.Key, fromID, toID); err != nil {
		return err
	}
	return tx.Commit()
}
