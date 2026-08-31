package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

var (
	ErrInvalidChannelWebhookSignature = errors.New("invalid channel Webhook signature")
	ErrChannelWebhookReplay           = errors.New("channel Webhook nonce was already used")
	ErrChannelWebhookIntegration      = errors.New("channel Webhook integration not found")
)

const channelWebhookClockSkew = 5 * time.Minute

func (s *DeliveryReceiptService) ProcessChannelWebhookCallback(
	ctx context.Context,
	callback domain.ChannelWebhookReceiptCallback,
) (*domain.DeliveryReceiptRecordResult, error) {
	if callback.WorkspaceID == "" || callback.IntegrationID == "" || callback.Nonce == "" || callback.Timestamp == 0 {
		return nil, domain.NewValidationError("workspace_id, integration_id, timestamp and nonce are required")
	}
	if s.channelWebhookNonces == nil {
		return nil, errors.New("channel Webhook nonce repository is not configured")
	}
	now := s.now().UTC()
	receivedAt := time.Unix(callback.Timestamp, 0).UTC()
	if receivedAt.Before(now.Add(-channelWebhookClockSkew)) || receivedAt.After(now.Add(channelWebhookClockSkew)) {
		return nil, ErrInvalidChannelWebhookSignature
	}
	workspace, err := s.workspaceRepo.GetByID(ctx, callback.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load channel Webhook callback workspace: %w", err)
	}
	integration := workspace.GetIntegrationByID(callback.IntegrationID)
	if integration == nil || integration.Type != domain.IntegrationTypeChannelWebhook ||
		integration.ChannelWebhookSettings == nil || integration.ChannelWebhookSettings.Secret == "" {
		return nil, ErrChannelWebhookIntegration
	}
	expected := SignChannelWebhook(integration.ChannelWebhookSettings.Secret, callback.Timestamp, callback.Nonce, callback.Body)
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(callback.Signature))) {
		return nil, ErrInvalidChannelWebhookSignature
	}
	reserved, err := s.channelWebhookNonces.Reserve(
		ctx, callback.WorkspaceID, callback.IntegrationID, callback.Nonce, now.Add(channelWebhookClockSkew),
	)
	if err != nil {
		return nil, err
	}
	if !reserved {
		return nil, ErrChannelWebhookReplay
	}

	decoder := json.NewDecoder(bytes.NewReader(callback.Body))
	decoder.DisallowUnknownFields()
	var payload domain.ChannelWebhookReceiptPayload
	if err := decoder.Decode(&payload); err != nil {
		return nil, domain.NewValidationError("invalid channel Webhook receipt: " + err.Error())
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, domain.NewValidationError("invalid channel Webhook receipt: exactly one JSON object is required")
	}
	receipt := domain.DeliveryReceipt{
		Provider: domain.DeliveryProviderChannelWebhook, ReceiptID: payload.ReceiptID,
		ProviderMessageID: payload.ProviderMessageID, MessageID: payload.MessageID, EffectKey: payload.EffectKey,
		Event: payload.Event, OccurredAt: payload.OccurredAt, ErrorCode: payload.ErrorCode, Metadata: payload.Metadata,
	}
	if err := receipt.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	receipt.PayloadHash, err = receipt.ComputePayloadHash()
	if err != nil {
		return nil, err
	}
	results, err := s.repo.RecordBatch(ctx, callback.WorkspaceID, []domain.DeliveryReceipt{receipt})
	if err != nil {
		return nil, fmt.Errorf("record channel Webhook delivery receipt: %w", err)
	}
	if len(results) != 1 {
		return nil, errors.New("record channel Webhook delivery receipt returned no result")
	}
	if results[0].Conflict {
		return &results[0], domain.ErrDeliveryReceiptPayloadConflict
	}
	return &results[0], nil
}
