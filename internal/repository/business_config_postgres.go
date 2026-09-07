package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
)

type BusinessConfigPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewBusinessConfigRepository(w domain.WorkspaceRepository) *BusinessConfigPostgresRepository {
	return &BusinessConfigPostgresRepository{workspaceRepo: w}
}
func NewBusinessConfigRepositoryWithDB(db *sql.DB) *BusinessConfigPostgresRepository {
	return &BusinessConfigPostgresRepository{db: db}
}
func (r *BusinessConfigPostgresRepository) getDB(ctx context.Context, w string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	return r.workspaceRepo.GetConnection(ctx, w)
}

const businessDefinitionColumns = `id,kind,key,name,description,schema,settings,status,revision,created_at,updated_at`

type businessScanner interface{ Scan(...any) error }

func scanBusinessDefinition(row businessScanner) (*domain.BusinessDefinition, error) {
	var d domain.BusinessDefinition
	var schema, settings []byte
	err := row.Scan(&d.ID, &d.Kind, &d.Key, &d.Name, &d.Description, &schema, &settings, &d.Status, &d.Revision, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrBusinessDefinitionNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(schema, &d.Schema); err != nil {
		return nil, err
	}
	err = json.Unmarshal(settings, &d.Settings)
	return &d, err
}
func (r *BusinessConfigPostgresRepository) Get(ctx context.Context, w, kind, key string) (*domain.BusinessDefinition, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	return scanBusinessDefinition(db.QueryRowContext(ctx, `SELECT `+businessDefinitionColumns+` FROM business_definitions WHERE kind=$1 AND key=$2`, kind, key))
}
func (r *BusinessConfigPostgresRepository) List(ctx context.Context, w string, f domain.BusinessDefinitionFilter) ([]domain.BusinessDefinition, int, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	where := ` WHERE ($1='' OR kind=$1) AND ($2='' OR status=$2) AND ($3='' OR key ILIKE '%'||$3||'%' OR name ILIKE '%'||$3||'%')`
	var total int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM business_definitions`+where, f.Kind, f.Status, f.Search).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+businessDefinitionColumns+` FROM business_definitions`+where+` ORDER BY kind,key LIMIT $4 OFFSET $5`, f.Kind, f.Status, f.Search, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.BusinessDefinition{}
	for rows.Next() {
		d, err := scanBusinessDefinition(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *d)
	}
	return out, total, rows.Err()
}
func (r *BusinessConfigPostgresRepository) Save(ctx context.Context, w string, d *domain.BusinessDefinition) (*domain.BusinessDefinition, error) {
	if err := d.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	db, err := r.getDB(ctx, w)
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
	if err = validateBusinessPublicationTx(ctx, tx, d); err != nil {
		return nil, err
	}
	schema, _ := json.Marshal(d.Schema)
	settings, _ := json.Marshal(d.Settings)
	var saved *domain.BusinessDefinition
	if d.ID == "" {
		saved, err = scanBusinessDefinition(tx.QueryRowContext(ctx, `INSERT INTO business_definitions(id,kind,key,name,description,schema,settings,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+businessDefinitionColumns, uuid.New(), d.Kind, d.Key, d.Name, d.Description, schema, settings, d.Status))
	} else {
		saved, err = scanBusinessDefinition(tx.QueryRowContext(ctx, `UPDATE business_definitions SET name=$4,description=$5,schema=$6,settings=$7,status=$8,revision=revision+1,updated_at=NOW() WHERE id=$1 AND kind=$2 AND key=$3 AND revision=$9 RETURNING `+businessDefinitionColumns, d.ID, d.Kind, d.Key, d.Name, d.Description, schema, settings, d.Status, d.Revision))
	}
	if err != nil {
		var pg *pq.Error
		if errors.Is(err, domain.ErrBusinessDefinitionNotFound) || (errors.As(err, &pg) && pg.Code == "23505") {
			return nil, domain.ErrBusinessDefinitionConflict
		}
		return nil, err
	}
	document, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO business_definition_revisions(definition_id,revision,document) VALUES($1,$2,$3)`, saved.ID, saved.Revision, document); err != nil {
		return nil, fmt.Errorf("save definition revision: %w", err)
	}
	if saved.Kind == "state" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id,next_check) SELECT 'customer',id::text,NOW() FROM customers WHERE merged_into_id IS NULL UNION ALL SELECT object_type,id::text,NOW() FROM business_objects ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=NOW()`); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}
