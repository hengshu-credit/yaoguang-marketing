package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ChannelSendPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

func NewChannelSendRepository(workspaceRepo domain.WorkspaceRepository) (*ChannelSendPostgresRepository, error) {
	if workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return &ChannelSendPostgresRepository{workspaceRepo: workspaceRepo}, nil
}

func (r *ChannelSendPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection: %w", err)
	}
	return db, nil
}

const channelSendFields = `effect_key, request_hash, message_id, channel, integration_id,
	contact_email, endpoint_id, template_id, template_version, language, status,
	provider, provider_message_id, attempts, last_error, created_at, updated_at`

func scanChannelSend(scanner interface{ Scan(...interface{}) error }) (domain.ChannelSendExecution, error) {
	var execution domain.ChannelSendExecution
	var language, provider, providerMessageID, lastError sql.NullString
	err := scanner.Scan(
		&execution.EffectKey, &execution.RequestHash, &execution.MessageID, &execution.Channel,
		&execution.IntegrationID, &execution.ContactEmail, &execution.EndpointID,
		&execution.TemplateID, &execution.TemplateVersion, &language, &execution.Status,
		&provider, &providerMessageID, &execution.Attempts, &lastError,
		&execution.CreatedAt, &execution.UpdatedAt,
	)
	execution.Language = language.String
	execution.Provider = provider.String
	execution.ProviderMessageID = providerMessageID.String
	execution.LastError = lastError.String
	return execution, err
}

func (r *ChannelSendPostgresRepository) Reserve(
	ctx context.Context,
	workspaceID string,
	execution domain.ChannelSendExecution,
) (domain.ChannelSendExecution, bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return domain.ChannelSendExecution{}, false, err
	}
	now := time.Now().UTC()
	if execution.Status == "" {
		execution.Status = domain.ChannelSendReserved
	}
	if execution.CreatedAt.IsZero() {
		execution.CreatedAt = now
	}
	if execution.UpdatedAt.IsZero() {
		execution.UpdatedAt = now
	}
	stored, err := scanChannelSend(db.QueryRowContext(ctx, `
		INSERT INTO channel_send_executions (`+channelSendFields+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,NULL,NULL,0,NULL,$12,$13)
		ON CONFLICT (effect_key) DO NOTHING
		RETURNING `+channelSendFields,
		execution.EffectKey, execution.RequestHash, execution.MessageID, execution.Channel,
		execution.IntegrationID, execution.ContactEmail, execution.EndpointID, execution.TemplateID,
		execution.TemplateVersion, execution.Language, execution.Status, execution.CreatedAt, execution.UpdatedAt,
	))
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.ChannelSendExecution{}, false, fmt.Errorf("reserve channel send: %w", err)
	}
	stored, err = scanChannelSend(db.QueryRowContext(ctx,
		`SELECT `+channelSendFields+` FROM channel_send_executions WHERE effect_key = $1`, execution.EffectKey))
	if err != nil {
		return domain.ChannelSendExecution{}, false, fmt.Errorf("load channel send: %w", err)
	}
	if stored.RequestHash != execution.RequestHash {
		return domain.ChannelSendExecution{}, false, domain.ErrChannelSendHashConflict
	}
	return stored, false, nil
}

func (r *ChannelSendPostgresRepository) MarkSubmitted(ctx context.Context, workspaceID, effectKey string, at time.Time) (bool, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `UPDATE channel_send_executions
		SET status = 'submitted', attempts = attempts + 1, updated_at = $2
		WHERE effect_key = $1 AND status = 'reserved'`, effectKey, at.UTC())
	if err != nil {
		return false, fmt.Errorf("mark channel send submitted: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *ChannelSendPostgresRepository) Fail(
	ctx context.Context,
	workspaceID, effectKey string,
	status domain.ChannelSendStatus,
	lastError string,
	at time.Time,
) (bool, error) {
	if status != domain.ChannelSendFailed && status != domain.ChannelSendUnknown {
		return false, fmt.Errorf("invalid terminal channel send status %q", status)
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `UPDATE channel_send_executions
		SET status = $3, last_error = LEFT($4, 1000), updated_at = $5
		WHERE effect_key = $1 AND status = $2`, effectKey, domain.ChannelSendSubmitted, status, lastError, at.UTC())
	if err != nil {
		return false, fmt.Errorf("fail channel send: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *ChannelSendPostgresRepository) Confirm(
	ctx context.Context,
	workspaceID, effectKey, provider, providerMessageID, secretKey string,
	message *domain.MessageHistory,
	at time.Time,
) (bool, error) {
	if message == nil || provider == "" || providerMessageID == "" {
		return false, errors.New("message, provider, and provider message id are required")
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin channel send confirmation: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE channel_send_executions
		SET status = 'confirmed', provider = $3, provider_message_id = $4,
			last_error = NULL, updated_at = $5
		WHERE effect_key = $1 AND status = $2`, effectKey, domain.ChannelSendSubmitted, provider, providerMessageID, at.UTC())
	if err != nil {
		return false, fmt.Errorf("confirm channel send: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows != 1 {
		return false, nil
	}
	encryptedData, err := encryptMessageData(message.MessageData, secretKey)
	if err != nil {
		return false, fmt.Errorf("encrypt channel message data: %w", err)
	}
	externalID := providerMessageID
	_, err = tx.ExecContext(ctx, `INSERT INTO message_history (
		id, external_id, contact_email, template_id, template_version, channel,
		status_info, message_data, channel_options, sent_at, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,$10)`,
		message.ID, externalID, message.ContactEmail, message.TemplateID, message.TemplateVersion,
		message.Channel, message.StatusInfo, encryptedData, message.ChannelOptions, message.SentAt.UTC())
	if err != nil {
		return false, fmt.Errorf("record confirmed channel message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit channel send confirmation: %w", err)
	}
	return true, nil
}
