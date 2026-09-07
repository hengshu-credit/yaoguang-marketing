package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type ExecutionSnapshotPostgresRepository struct {
	workspaces domain.WorkspaceRepository
	db         *sql.DB
}

func NewExecutionSnapshotRepository(workspaces domain.WorkspaceRepository) *ExecutionSnapshotPostgresRepository {
	return &ExecutionSnapshotPostgresRepository{workspaces: workspaces}
}
func NewExecutionSnapshotRepositoryWithDB(db *sql.DB) *ExecutionSnapshotPostgresRepository {
	return &ExecutionSnapshotPostgresRepository{db: db}
}
func (r *ExecutionSnapshotPostgresRepository) getDB(ctx context.Context, ws string) (*sql.DB, error) {
	if ws == "" {
		return nil, errors.New("workspace id is required")
	}
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaces == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaces.GetConnection(ctx, ws)
}

type executionSQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func saveExecutionSnapshot(ctx context.Context, db executionSQL, s *domain.ExecutionSnapshot) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.Missing == nil {
		s.Missing = []string{}
	}
	if err := s.Rehash(); err != nil {
		return err
	}
	missing, _ := json.Marshal(s.Missing)
	result, err := db.ExecContext(ctx, `INSERT INTO execution_config_snapshots (id,execution_id,kind,source_id,configuration,materials,dependencies,content_hash,completeness,missing,created_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(execution_id) DO NOTHING`, s.ID, s.ExecutionID, s.Kind, s.SourceID, []byte(s.Configuration), []byte(s.Materials), []byte(s.Dependencies), s.ContentHash, s.Completeness, missing, s.CreatedAt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	var hash string
	if err = db.QueryRowContext(ctx, `SELECT content_hash FROM execution_config_snapshots WHERE execution_id=$1`, s.ExecutionID).Scan(&hash); err != nil {
		return err
	}
	if hash != s.ContentHash {
		return domain.ErrExecutionSnapshotConflict
	}
	return nil
}
func (r *ExecutionSnapshotPostgresRepository) Save(ctx context.Context, ws string, s *domain.ExecutionSnapshot) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	return saveExecutionSnapshot(ctx, db, s)
}

const executionSnapshotColumns = `id,execution_id,kind,source_id,configuration,materials,dependencies,content_hash,completeness,missing,created_at`

type executionScanner interface{ Scan(...any) error }

func scanExecutionSnapshot(row executionScanner) (*domain.ExecutionSnapshot, error) {
	s := &domain.ExecutionSnapshot{}
	var missing []byte
	err := row.Scan(&s.ID, &s.ExecutionID, &s.Kind, &s.SourceID, &s.Configuration, &s.Materials, &s.Dependencies, &s.ContentHash, &s.Completeness, &missing, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrExecutionSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(missing, &s.Missing); err != nil {
		return nil, err
	}
	return s, nil
}
func (r *ExecutionSnapshotPostgresRepository) Get(ctx context.Context, ws, id string) (*domain.ExecutionSnapshot, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	return scanExecutionSnapshot(db.QueryRowContext(ctx, `SELECT `+executionSnapshotColumns+` FROM execution_config_snapshots WHERE execution_id=$1 OR execution_id=(SELECT id::text FROM journey_instances WHERE contact_automation_id=$1)`, id))
}
func (r *ExecutionSnapshotPostgresRepository) List(ctx context.Context, ws, source string, limit int) ([]domain.ExecutionSnapshot, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT `+executionSnapshotColumns+` FROM execution_config_snapshots WHERE (source_id=$1 OR source_id IN(SELECT id::text FROM campaigns WHERE source_type='broadcast' AND source_id=$1)) ORDER BY created_at DESC,id DESC LIMIT $2`, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ExecutionSnapshot{}
	for rows.Next() {
		s, err := scanExecutionSnapshot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *s)
	}
	return items, rows.Err()
}
func (r *ExecutionSnapshotPostgresRepository) AppendEvidence(ctx context.Context, ws string, e domain.ExecutionEvidence) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	return appendExecutionEvidence(ctx, db, e)
}

type executionEvidenceDB interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func appendExecutionEvidence(ctx context.Context, db executionEvidenceDB, e domain.ExecutionEvidence) error {
	if err := e.Validate(); err != nil {
		return err
	}
	payload, err := domain.SanitizeExecutionJSON(e.Payload)
	if err != nil {
		return err
	}
	e.Payload = payload
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	body, _ := json.Marshal([]any{e.Kind, e.NodeID, e.CustomerID, e.DeliveryID, e.Status, e.Payload})
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	result, err := db.ExecContext(ctx, `INSERT INTO execution_evidence(id,execution_id,evidence_key,kind,node_id,customer_id,delivery_id,status,payload,payload_hash,occurred_at)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11) ON CONFLICT(execution_id,evidence_key) DO NOTHING`, e.ID, e.ExecutionID, e.Key, e.Kind, e.NodeID, e.CustomerID, e.DeliveryID, e.Status, []byte(e.Payload), hash, e.OccurredAt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	var existing string
	if err = db.QueryRowContext(ctx, `SELECT payload_hash FROM execution_evidence WHERE execution_id=$1 AND evidence_key=$2`, e.ExecutionID, e.Key).Scan(&existing); err != nil {
		return err
	}
	if existing != hash {
		return domain.ErrExecutionSnapshotConflict
	}
	return nil
}
func (r *ExecutionSnapshotPostgresRepository) ListEvidence(ctx context.Context, ws, id, after string, limit int) ([]domain.ExecutionEvidence, string, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, "", err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `SELECT id,execution_id,evidence_key,kind,node_id,COALESCE(customer_id::text,''),delivery_id,status,payload,occurred_at
 FROM execution_evidence WHERE execution_id=$1 AND ($2='' OR (occurred_at,id) > (SELECT occurred_at,id FROM execution_evidence WHERE id::text=$2 AND execution_id=$1)) ORDER BY occurred_at,id LIMIT $3`, id, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []domain.ExecutionEvidence{}
	for rows.Next() {
		var e domain.ExecutionEvidence
		if err = rows.Scan(&e.ID, &e.ExecutionID, &e.Key, &e.Kind, &e.NodeID, &e.CustomerID, &e.DeliveryID, &e.Status, &e.Payload, &e.OccurredAt); err != nil {
			return nil, "", err
		}
		items = append(items, e)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[limit-1].ID
	}
	return items, next, rows.Err()
}
func (r *ExecutionSnapshotPostgresRepository) SaveAsset(ctx context.Context, ws string, a domain.ExecutionAsset) error {
	sum := sha256.Sum256(a.Data)
	if a.Hash != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("asset hash mismatch")
	}
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO execution_archived_assets(hash,media_type,data) VALUES($1,$2,$3) ON CONFLICT(hash) DO NOTHING`, a.Hash, a.MediaType, a.Data)
	return err
}
func (r *ExecutionSnapshotPostgresRepository) GetAsset(ctx context.Context, ws, hash string) (*domain.ExecutionAsset, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	a := &domain.ExecutionAsset{}
	err = db.QueryRowContext(ctx, `SELECT hash,media_type,data FROM execution_archived_assets WHERE hash=$1`, hash).Scan(&a.Hash, &a.MediaType, &a.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrExecutionSnapshotNotFound
	}
	return a, err
}

func (r *ExecutionSnapshotPostgresRepository) SaveTemplateMaterialArchive(ctx context.Context, ws string, template *domain.Template, assets json.RawMessage) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	translations := template.Translations
	if translations == nil {
		translations = map[string]domain.TemplateTranslation{}
	}
	content, err := domain.SanitizeExecutionJSON(map[string]any{"channel": template.Channel, "email": template.Email, "web": template.Web, "sms": template.SMS, "push": template.Push, "content": template.Content, "translations": translations, "settings": template.Settings, "template_macro_id": template.TemplateMacroID})
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO execution_template_material_archives(template_id,content_hash,content,assets) VALUES($1,encode(sha256(convert_to($2::jsonb::text,'UTF8')),'hex'),$2::jsonb,$3::jsonb) ON CONFLICT(template_id,content_hash) DO NOTHING`, template.ID, []byte(content), []byte(assets))
	return err
}
