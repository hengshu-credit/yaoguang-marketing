package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	domainmocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWebhookNonceRepository struct{ reserved bool }

func (r *fakeWebhookNonceRepository) Reserve(context.Context, string, string, string, time.Time) (bool, error) {
	return r.reserved, nil
}

type fakeWebhookReceiptRepository struct{ receipts []domain.DeliveryReceipt }

func (r *fakeWebhookReceiptRepository) RecordBatch(_ context.Context, _ string, receipts []domain.DeliveryReceipt) ([]domain.DeliveryReceiptRecordResult, error) {
	r.receipts = append(r.receipts, receipts...)
	return []domain.DeliveryReceiptRecordResult{{Provider: receipts[0].Provider, ReceiptID: receipts[0].ReceiptID, Applied: true}}, nil
}

func TestProcessChannelWebhookReceiptVerifiesSignatureNonceAndNormalizes(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := domainmocks.NewMockAuthService(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	receipts := &fakeWebhookReceiptRepository{}
	service, err := NewDeliveryReceiptService(auth, receipts, workspaceRepo, 500)
	require.NoError(t, err)
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.SetChannelWebhookNonceRepository(&fakeWebhookNonceRepository{reserved: true})
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{{
		ID: "bridge-1", Type: domain.IntegrationTypeChannelWebhook,
		ChannelWebhookSettings: &domain.ChannelWebhookSettings{Secret: "plain-secret", Channels: []string{"telegram"}},
	}}}, nil)

	payload := domain.ChannelWebhookReceiptPayload{
		ReceiptID: "receipt-1", ProviderMessageID: "provider-1", EffectKey: "effect-1",
		Event: domain.DeliveryReceiptDelivered, OccurredAt: time.Date(2026, 8, 31, 11, 59, 0, 0, time.UTC),
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	timestamp := now.Unix()
	result, err := service.ProcessChannelWebhookCallback(context.Background(), domain.ChannelWebhookReceiptCallback{
		WorkspaceID: "ws-1", IntegrationID: "bridge-1", Timestamp: timestamp, Nonce: "nonce-1",
		Signature: SignChannelWebhook("plain-secret", timestamp, "nonce-1", body), Body: body,
	})
	require.NoError(t, err)
	assert.True(t, result.Applied)
	require.Len(t, receipts.receipts, 1)
	assert.Equal(t, domain.DeliveryProviderChannelWebhook, receipts.receipts[0].Provider)
	assert.Equal(t, domain.DeliveryReceiptDelivered, receipts.receipts[0].Event)
}

func TestProcessChannelWebhookReceiptRejectsReplay(t *testing.T) {
	ctrl := gomock.NewController(t)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	service, err := NewDeliveryReceiptService(domainmocks.NewMockAuthService(ctrl), &fakeWebhookReceiptRepository{}, workspaceRepo, 500)
	require.NoError(t, err)
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.SetChannelWebhookNonceRepository(&fakeWebhookNonceRepository{reserved: false})
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1", Integrations: domain.Integrations{{
		ID: "bridge-1", Type: domain.IntegrationTypeChannelWebhook,
		ChannelWebhookSettings: &domain.ChannelWebhookSettings{Secret: "plain-secret", Channels: []string{"telegram"}},
	}}}, nil)
	body, err := json.Marshal(domain.ChannelWebhookReceiptPayload{ReceiptID: "receipt-1", EffectKey: "effect-1", Event: domain.DeliveryReceiptDelivered, OccurredAt: now})
	require.NoError(t, err)
	_, err = service.ProcessChannelWebhookCallback(context.Background(), domain.ChannelWebhookReceiptCallback{
		WorkspaceID: "ws-1", IntegrationID: "bridge-1", Timestamp: now.Unix(), Nonce: "nonce-1",
		Signature: SignChannelWebhook("plain-secret", now.Unix(), "nonce-1", body), Body: body,
	})
	assert.ErrorIs(t, err, ErrChannelWebhookReplay)
}
