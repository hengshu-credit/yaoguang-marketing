package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

type DeliveryReceiptRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

func NewDeliveryReceiptRepository(workspaceRepo domain.WorkspaceRepository) *DeliveryReceiptRepository {
	return &DeliveryReceiptRepository{workspaceRepo: workspaceRepo}
}

func (r *DeliveryReceiptRepository) RecordBatch(
	ctx context.Context,
	workspaceID string,
	receipts []domain.DeliveryReceipt,
) ([]domain.DeliveryReceiptRecordResult, error) {
	if len(receipts) == 0 {
		return []domain.DeliveryReceiptRecordResult{}, nil
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection for delivery receipts: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin delivery receipt transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	results := make([]domain.DeliveryReceiptRecordResult, len(receipts))
	receivedAt := time.Now().UTC()
	for index := range receipts {
		result, err := recordDeliveryReceiptTx(ctx, tx, receipts[index], receivedAt)
		if err != nil {
			return nil, fmt.Errorf("record delivery receipt at index %d: %w", index, err)
		}
		results[index] = result
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit delivery receipt transaction: %w", err)
	}
	return results, nil
}

func recordDeliveryReceiptTx(
	ctx context.Context,
	tx *sql.Tx,
	receipt domain.DeliveryReceipt,
	receivedAt time.Time,
) (domain.DeliveryReceiptRecordResult, error) {
	result := domain.DeliveryReceiptRecordResult{Provider: receipt.Provider, ReceiptID: receipt.ReceiptID}
	if receipt.PayloadHash == "" {
		hash, err := receipt.ComputePayloadHash()
		if err != nil {
			return result, err
		}
		receipt.PayloadHash = hash
	}
	metadataValue := receipt.Metadata
	if metadataValue == nil {
		metadataValue = map[string]interface{}{}
	}
	metadata, err := json.Marshal(metadataValue)
	if err != nil {
		return result, fmt.Errorf("marshal receipt metadata: %w", err)
	}

	var storedReceivedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO delivery_receipts (
			provider, receipt_id, provider_message_id, message_id, effect_key,
			event, occurred_at, received_at, error_code, metadata, payload_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (provider, receipt_id) DO NOTHING
		RETURNING received_at
	`,
		receipt.Provider, receipt.ReceiptID, receipt.ProviderMessageID,
		nullableString(receipt.MessageID), nullableString(receipt.EffectKey), receipt.Event,
		receipt.OccurredAt, receivedAt, nullableString(receipt.ErrorCode), metadata, receipt.PayloadHash,
	).Scan(&storedReceivedAt)

	storedMessageID := sql.NullString{String: receipt.MessageID, Valid: receipt.MessageID != ""}
	if errors.Is(err, sql.ErrNoRows) {
		result.Duplicate = true
		var existingHash string
		if lookupErr := tx.QueryRowContext(ctx, `
			SELECT payload_hash, message_id
			FROM delivery_receipts
			WHERE provider = $1 AND receipt_id = $2
			FOR UPDATE
		`, receipt.Provider, receipt.ReceiptID).Scan(&existingHash, &storedMessageID); lookupErr != nil {
			return result, fmt.Errorf("load existing delivery receipt: %w", lookupErr)
		}
		if existingHash != receipt.PayloadHash {
			result.Conflict = true
			return result, nil
		}
	} else if err != nil {
		return result, fmt.Errorf("insert delivery receipt: %w", err)
	}

	messageID := ""
	if storedMessageID.Valid {
		messageID = storedMessageID.String
	}
	if messageID == "" {
		messageID, err = resolveReceiptMessageID(ctx, tx, receipt)
		if err != nil {
			return result, err
		}
	}
	if messageID == "" {
		return result, nil
	}

	result.MessageID = messageID
	result.Matched = true
	if !storedMessageID.Valid || storedMessageID.String != messageID {
		if _, err := tx.ExecContext(ctx,
			`UPDATE delivery_receipts SET message_id = $1 WHERE provider = $2 AND receipt_id = $3`,
			messageID, receipt.Provider, receipt.ReceiptID,
		); err != nil {
			return result, fmt.Errorf("link delivery receipt to message: %w", err)
		}
	}

	field := deliveryReceiptStatusField(receipt.Event)
	if field == "" {
		return result, nil
	}
	statusInfo := nullableString(receipt.ErrorCode)
	query := fmt.Sprintf(`
		UPDATE message_history
		SET %s = $1,
			status_info = COALESCE(LEFT($2, 255), status_info),
			updated_at = NOW()
		WHERE id = $3 AND %s IS NULL
	`, field, field)
	execution, err := tx.ExecContext(ctx, query, receipt.OccurredAt, statusInfo, messageID)
	if err != nil {
		return result, fmt.Errorf("apply %s delivery receipt: %w", receipt.Event, err)
	}
	rows, err := execution.RowsAffected()
	if err != nil {
		return result, fmt.Errorf("read applied delivery receipt row count: %w", err)
	}
	result.Applied = rows > 0
	return result, nil
}

func resolveReceiptMessageID(ctx context.Context, tx *sql.Tx, receipt domain.DeliveryReceipt) (string, error) {
	var messageID string
	if receipt.MessageID != "" {
		err := tx.QueryRowContext(ctx, `SELECT id FROM message_history WHERE id = $1`, receipt.MessageID).Scan(&messageID)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("match delivery receipt message_id: %w", err)
		}
		return messageID, nil
	}
	if receipt.ProviderMessageID == "" {
		return "", nil
	}
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM message_history WHERE external_id = $1 ORDER BY created_at DESC LIMIT 1`,
		receipt.ProviderMessageID,
	).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("match delivery receipt provider_message_id: %w", err)
	}
	return messageID, nil
}

func deliveryReceiptStatusField(event domain.DeliveryReceiptEvent) string {
	switch event {
	case domain.DeliveryReceiptDelivered:
		return "delivered_at"
	case domain.DeliveryReceiptOpened:
		return "opened_at"
	case domain.DeliveryReceiptFailed:
		return "failed_at"
	default:
		return ""
	}
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
