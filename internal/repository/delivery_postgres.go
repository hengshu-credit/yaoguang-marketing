package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type DeliveryPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
	queueRepo     domain.EmailQueueRepository
}

func NewDeliveryRepository(workspaceRepo domain.WorkspaceRepository, queueRepo domain.EmailQueueRepository) domain.DeliveryRepository {
	return &DeliveryPostgresRepository{workspaceRepo: workspaceRepo, queueRepo: queueRepo}
}

func NewDeliveryRepositoryWithDB(db *sql.DB) domain.DeliveryRepository {
	return &DeliveryPostgresRepository{db: db, queueRepo: &EmailQueueRepository{db: db}}
}

func (r *DeliveryPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

const deliveryIntentColumnsSQL = `id, effect_key, request_hash, source_type, source_id,
	source_version, customer_id, legacy_identity, channel, template_id, template_version,
	node_or_phase, occurrence, variant, status, suppression_reason, metadata, created_at, updated_at`

const insertDeliveryIntentSQL = `INSERT INTO delivery_intents (
	id, effect_key, request_hash, source_type, source_id, source_version,
	customer_id, legacy_identity, channel, template_id, template_version,
	node_or_phase, occurrence, variant, status, suppression_reason, metadata
) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid, NULLIF($8, ''),
	$9, NULLIF($10, ''), NULLIF($11, 0), $12, $13, $14, $15, NULLIF($16, ''), $17)
ON CONFLICT (effect_key) DO NOTHING
RETURNING ` + deliveryIntentColumnsSQL

const selectDeliveryIntentByEffectKeyForUpdateSQL = `SELECT ` + deliveryIntentColumnsSQL + `
	FROM delivery_intents WHERE effect_key = $1 FOR UPDATE`

const selectDeliveryIntentByEffectKeySQL = `SELECT ` + deliveryIntentColumnsSQL + `
	FROM delivery_intents WHERE effect_key = $1`

type deliveryRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanDeliveryIntent(row deliveryRowScanner) (domain.DeliveryIntent, error) {
	var intent domain.DeliveryIntent
	var customerID, legacyIdentity, templateID, suppressionReason sql.NullString
	var templateVersion sql.NullInt64
	var metadata []byte
	err := row.Scan(
		&intent.ID, &intent.EffectKey, &intent.RequestHash, &intent.SourceType, &intent.SourceID,
		&intent.SourceVersion, &customerID, &legacyIdentity, &intent.Channel, &templateID,
		&templateVersion, &intent.NodeOrPhase, &intent.Occurrence, &intent.Variant,
		&intent.Status, &suppressionReason, &metadata, &intent.CreatedAt, &intent.UpdatedAt,
	)
	if err != nil {
		return domain.DeliveryIntent{}, err
	}
	if customerID.Valid {
		intent.CustomerID = customerID.String
	}
	if legacyIdentity.Valid {
		intent.LegacyIdentity = legacyIdentity.String
	}
	if templateID.Valid {
		intent.TemplateID = templateID.String
	}
	if templateVersion.Valid {
		intent.TemplateVersion = templateVersion.Int64
	}
	if suppressionReason.Valid {
		intent.SuppressionReason = suppressionReason.String
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &intent.Metadata); err != nil {
			return domain.DeliveryIntent{}, fmt.Errorf("decode delivery intent metadata: %w", err)
		}
	}
	return intent, nil
}

func reserveDeliveryIntentTx(ctx context.Context, tx *sql.Tx, intent domain.DeliveryIntent) (domain.DeliveryIntent, bool, error) {
	if intent.EffectKey == "" || intent.RequestHash == "" {
		return domain.DeliveryIntent{}, false, errors.New("delivery effect key and request hash are required")
	}
	if intent.ID == "" {
		intent.ID = uuid.New().String()
	}
	if intent.Status == "" {
		intent.Status = domain.DeliveryStatusReserved
	}
	if intent.Status != domain.DeliveryStatusReserved {
		return domain.DeliveryIntent{}, false, errors.New("new delivery intent must start reserved")
	}
	metadata, err := json.Marshal(intent.Metadata)
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("encode delivery intent metadata: %w", err)
	}
	stored, scanErr := scanDeliveryIntent(tx.QueryRowContext(ctx, insertDeliveryIntentSQL,
		intent.ID, intent.EffectKey, intent.RequestHash, intent.SourceType, intent.SourceID,
		intent.SourceVersion, intent.CustomerID, intent.LegacyIdentity, intent.Channel, intent.TemplateID,
		intent.TemplateVersion, intent.NodeOrPhase, intent.Occurrence, intent.Variant, intent.Status,
		intent.SuppressionReason, metadata,
	))
	if scanErr == nil {
		return stored, true, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return domain.DeliveryIntent{}, false, fmt.Errorf("reserve delivery intent: %w", scanErr)
	}

	stored, err = scanDeliveryIntent(tx.QueryRowContext(ctx, selectDeliveryIntentByEffectKeyForUpdateSQL, intent.EffectKey))
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("load conflicting delivery intent: %w", err)
	}
	if stored.RequestHash != intent.RequestHash {
		return domain.DeliveryIntent{}, false, domain.ErrDeliveryIntentHashConflict
	}
	return stored, false, nil
}

func (r *DeliveryPostgresRepository) ReserveIntent(ctx context.Context, workspaceID string, intent domain.DeliveryIntent) (domain.DeliveryIntent, bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("begin delivery reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stored, created, err := reserveDeliveryIntentTx(ctx, tx, intent)
	if err != nil {
		return domain.DeliveryIntent{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.DeliveryIntent{}, false, fmt.Errorf("commit delivery reservation: %w", err)
	}
	return stored, created, nil
}

func (r *DeliveryPostgresRepository) ReserveAndEnqueue(ctx context.Context, workspaceID string, intent domain.DeliveryIntent, entry *domain.EmailQueueEntry) (domain.ReserveDeliveryResult, error) {
	var result domain.ReserveDeliveryResult
	if entry == nil {
		return result, errors.New("email queue entry is required")
	}
	if r.queueRepo == nil {
		return result, errors.New("email queue repository is required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return result, fmt.Errorf("get workspace database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin reserve and enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stored, created, err := reserveDeliveryIntentTx(ctx, tx, intent)
	if err != nil {
		return result, err
	}
	result.Intent, result.Created = stored, created

	if stored.Status == domain.DeliveryStatusReserved || stored.Status == domain.DeliveryStatusTransientFailed {
		entry.DeliveryIntentID = stored.ID
		queueCreated, enqueueErr := r.queueRepo.EnqueueIntentTx(ctx, tx, entry)
		if enqueueErr != nil {
			return result, enqueueErr
		}
		result.QueueCreated = queueCreated
		transition, updateErr := tx.ExecContext(ctx, `UPDATE delivery_intents
			SET status = 'queued', updated_at = CURRENT_TIMESTAMP
			WHERE id = $1 AND status IN ('reserved', 'transient_failed')`, stored.ID)
		if updateErr != nil {
			return result, fmt.Errorf("mark delivery intent queued: %w", updateErr)
		}
		rows, rowsErr := transition.RowsAffected()
		if rowsErr != nil {
			return result, fmt.Errorf("inspect delivery intent queue transition: %w", rowsErr)
		}
		if rows != 1 {
			return result, errors.New("delivery intent queue transition was rejected")
		}
		result.Intent.Status = domain.DeliveryStatusQueued
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit reserve and enqueue: %w", err)
	}
	return result, nil
}

func (r *DeliveryPostgresRepository) GetIntentByEffectKey(ctx context.Context, workspaceID, effectKey string) (*domain.DeliveryIntent, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace database: %w", err)
	}
	intent, err := scanDeliveryIntent(db.QueryRowContext(ctx, selectDeliveryIntentByEffectKeySQL, effectKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery intent by effect key: %w", err)
	}
	return &intent, nil
}

func (r *DeliveryPostgresRepository) ResolveCustomerID(ctx context.Context, workspaceID, email string) (string, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("get workspace database: %w", err)
	}
	var customerID sql.NullString
	err = db.QueryRowContext(ctx, `SELECT customer_id::text FROM contacts
		WHERE LOWER(BTRIM(email)) = LOWER(BTRIM($1::text))`, email).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve delivery customer by email: %w", err)
	}
	if customerID.Valid {
		return customerID.String, nil
	}
	return "", nil
}

func (r *DeliveryPostgresRepository) TransitionIntent(ctx context.Context, workspaceID, intentID string, from, to domain.DeliveryStatus, at time.Time) (bool, error) {
	if !from.CanTransitionTo(to) {
		return false, fmt.Errorf("delivery status transition %s to %s is not allowed", from, to)
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("get workspace database: %w", err)
	}
	result, err := db.ExecContext(ctx, `UPDATE delivery_intents SET status = $3, updated_at = $4
		WHERE id = $1 AND status = $2`, intentID, from, to, at.UTC())
	if err != nil {
		return false, fmt.Errorf("transition delivery intent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect delivery intent transition: %w", err)
	}
	return rows == 1, nil
}
