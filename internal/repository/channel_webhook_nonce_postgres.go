package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ChannelWebhookNonceRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

func NewChannelWebhookNonceRepository(workspaceRepo domain.WorkspaceRepository) *ChannelWebhookNonceRepository {
	return &ChannelWebhookNonceRepository{workspaceRepo: workspaceRepo}
}

func (r *ChannelWebhookNonceRepository) Reserve(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	nonce string,
	expiresAt time.Time,
) (bool, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("get workspace connection for channel Webhook nonce: %w", err)
	}
	var reserved bool
	err = db.QueryRowContext(ctx, `
		WITH expired AS (
			SELECT ctid FROM channel_webhook_nonces WHERE expires_at < NOW() LIMIT 100
		), deleted AS (
			DELETE FROM channel_webhook_nonces target USING expired
			WHERE target.ctid = expired.ctid
		), inserted AS (
			INSERT INTO channel_webhook_nonces (integration_id, nonce, expires_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (integration_id, nonce) DO NOTHING
			RETURNING TRUE AS reserved
		)
		SELECT EXISTS(SELECT 1 FROM inserted)
	`, integrationID, nonce, expiresAt.UTC()).Scan(&reserved)
	if err != nil {
		return false, fmt.Errorf("reserve channel Webhook nonce: %w", err)
	}
	return reserved, nil
}
