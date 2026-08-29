package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"sort"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type receiptRepositoryStub struct {
	receipts []domain.DeliveryReceipt
	results  []domain.DeliveryReceiptRecordResult
	err      error
}

func (r *receiptRepositoryStub) RecordBatch(_ context.Context, _ string, receipts []domain.DeliveryReceipt) ([]domain.DeliveryReceiptRecordResult, error) {
	r.receipts = append([]domain.DeliveryReceipt(nil), receipts...)
	return r.results, r.err
}

func TestDeliveryReceiptServiceIngestAuthenticatesOnceAndReportsResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := &receiptRepositoryStub{results: []domain.DeliveryReceiptRecordResult{{
		Provider: domain.DeliveryProviderFCM, ReceiptID: "receipt-1", Duplicate: true, Matched: true,
	}}}
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace-1").
		Return(ctx, &domain.User{ID: "user-1"}, &domain.UserWorkspace{Role: "owner", WorkspaceID: "workspace-1"}, nil)

	service, err := NewDeliveryReceiptService(auth, repo, workspaceRepo, 500)
	require.NoError(t, err)
	response, err := service.Ingest(ctx, &domain.IngestDeliveryReceiptsRequest{
		WorkspaceID: "workspace-1",
		Receipts: []domain.DeliveryReceipt{{
			Provider: domain.DeliveryProviderFCM, ReceiptID: "receipt-1",
			ProviderMessageID: "provider-message", Event: domain.DeliveryReceiptAccepted,
			OccurredAt: time.Now().UTC(),
		}},
	})
	require.NoError(t, err)
	require.Len(t, repo.receipts, 1)
	assert.Len(t, repo.receipts[0].PayloadHash, 64)
	assert.Equal(t, 1, response.Duplicates)
	assert.Equal(t, "duplicate", response.Results[0].Status)
}

func TestDeliveryReceiptServiceIngestKeepsPerItemValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := &receiptRepositoryStub{results: []domain.DeliveryReceiptRecordResult{{
		Provider: domain.DeliveryProviderFCM, ReceiptID: "valid",
	}}}
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace-1").
		Return(ctx, &domain.User{}, &domain.UserWorkspace{Role: "owner"}, nil)
	service, err := NewDeliveryReceiptService(auth, repo, workspaceRepo, 500)
	require.NoError(t, err)
	response, err := service.Ingest(ctx, &domain.IngestDeliveryReceiptsRequest{
		WorkspaceID: "workspace-1",
		Receipts: []domain.DeliveryReceipt{
			{Provider: "bad", ReceiptID: "invalid"},
			{Provider: domain.DeliveryProviderFCM, ReceiptID: "valid", ProviderMessageID: "message", Event: domain.DeliveryReceiptSent, OccurredAt: time.Now().UTC()},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Failed)
	assert.Equal(t, 1, response.Accepted)
	assert.Equal(t, "error", response.Results[0].Status)
	assert.Equal(t, "accepted", response.Results[1].Status)
}

func TestDeliveryReceiptServiceProcessesSignedTwilioCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := &receiptRepositoryStub{results: []domain.DeliveryReceiptRecordResult{{
		Provider: domain.DeliveryProviderTwilio, ReceiptID: "generated", Matched: true, Applied: true,
	}}}
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-1").Return(&domain.Workspace{
		ID: "workspace-1",
		Integrations: domain.Integrations{{
			ID: "sms-1", Type: domain.IntegrationTypeSMS,
			SMSProvider: &domain.SMSProvider{Kind: domain.SMSProviderKindTwilio, Twilio: &domain.TwilioSettings{AuthToken: "auth-secret"}},
		}},
	}, nil)
	callbackURL := "https://notify.example/webhooks/delivery/twilio?workspace_id=workspace-1&integration_id=sms-1&message_id=message-1"
	form := map[string][]string{
		"MessageSid":    {"SM111"},
		"MessageStatus": {"delivered"},
		"ErrorCode":     {""},
	}
	signature := signTwilioTestRequest(callbackURL, form, "auth-secret")

	service, err := NewDeliveryReceiptService(auth, repo, workspaceRepo, 500)
	require.NoError(t, err)
	result, err := service.ProcessTwilioCallback(context.Background(), domain.TwilioDeliveryCallback{
		WorkspaceID: "workspace-1", IntegrationID: "sms-1", CallbackURL: callbackURL,
		Signature: signature, MessageID: "message-1", Form: form,
	})
	require.NoError(t, err)
	assert.True(t, result.Applied)
	require.Len(t, repo.receipts, 1)
	assert.Equal(t, domain.DeliveryReceiptDelivered, repo.receipts[0].Event)
	assert.Equal(t, "SM111", repo.receipts[0].ProviderMessageID)
	assert.Equal(t, "message-1", repo.receipts[0].MessageID)
	assert.Len(t, repo.receipts[0].ReceiptID, 64)
}

func TestDeliveryReceiptServiceRejectsInvalidTwilioSignature(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := &receiptRepositoryStub{}
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-1").Return(&domain.Workspace{
		Integrations: domain.Integrations{{ID: "sms-1", Type: domain.IntegrationTypeSMS, SMSProvider: &domain.SMSProvider{
			Kind: domain.SMSProviderKindTwilio, Twilio: &domain.TwilioSettings{AuthToken: "auth-secret"},
		}}},
	}, nil)
	service, err := NewDeliveryReceiptService(auth, repo, workspaceRepo, 500)
	require.NoError(t, err)
	_, err = service.ProcessTwilioCallback(context.Background(), domain.TwilioDeliveryCallback{
		WorkspaceID: "workspace-1", IntegrationID: "sms-1", CallbackURL: "https://notify.example/callback",
		Signature: "invalid", Form: map[string][]string{"MessageSid": {"SM111"}, "MessageStatus": {"delivered"}},
	})
	assert.ErrorIs(t, err, ErrInvalidTwilioSignature)
	assert.Empty(t, repo.receipts)
}

func signTwilioTestRequest(callbackURL string, form map[string][]string, token string) string {
	keys := make([]string, 0, len(form))
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	data := callbackURL
	for _, key := range keys {
		values := append([]string(nil), form[key]...)
		sort.Strings(values)
		for _, value := range values {
			data += key + value
		}
	}
	mac := hmac.New(sha1.New, []byte(token))
	_, _ = mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
