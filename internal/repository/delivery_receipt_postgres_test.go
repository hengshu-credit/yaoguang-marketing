package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryReceiptRepositoryRecordBatchInsertsMatchesAndApplies(t *testing.T) {
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-1").Return(db, nil)

	occurredAt := time.Date(2026, 8, 29, 8, 30, 0, 0, time.UTC)
	receipt := domain.DeliveryReceipt{
		Provider: domain.DeliveryProviderTwilio, ReceiptID: "receipt-1",
		ProviderMessageID: "SM111", Event: domain.DeliveryReceiptDelivered,
		OccurredAt: occurredAt, PayloadHash: "hash-1",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO delivery_receipts")).
		WithArgs("twilio", "receipt-1", "SM111", nil, nil, "delivered", occurredAt, sqlmock.AnyArg(), nil, sqlmock.AnyArg(), "hash-1").
		WillReturnRows(sqlmock.NewRows([]string{"received_at"}).AddRow(occurredAt))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM message_history WHERE external_id = $1 ORDER BY created_at DESC LIMIT 1")).
		WithArgs("SM111").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("message-1"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE delivery_receipts SET message_id = $1 WHERE provider = $2 AND receipt_id = $3")).
		WithArgs("message-1", "twilio", "receipt-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE message_history").
		WithArgs(occurredAt, nil, "message-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewDeliveryReceiptRepository(workspaceRepo)
	results, err := repo.RecordBatch(context.Background(), "workspace-1", []domain.DeliveryReceipt{receipt})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "message-1", results[0].MessageID)
	assert.True(t, results[0].Matched)
	assert.True(t, results[0].Applied)
	assert.False(t, results[0].Duplicate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveryReceiptRepositoryRecordBatchReportsPayloadConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-1").Return(db, nil)

	receipt := domain.DeliveryReceipt{
		Provider: domain.DeliveryProviderFCM, ReceiptID: "receipt-1",
		ProviderMessageID: "provider-message", Event: domain.DeliveryReceiptAccepted,
		OccurredAt: time.Now().UTC(), PayloadHash: "new-hash",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO delivery_receipts")).
		WillReturnRows(sqlmock.NewRows([]string{"received_at"}))
	mock.ExpectQuery("SELECT payload_hash, message_id").
		WithArgs("fcm", "receipt-1").
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "message_id"}).AddRow("old-hash", nil))
	mock.ExpectCommit()

	repo := NewDeliveryReceiptRepository(workspaceRepo)
	results, err := repo.RecordBatch(context.Background(), "workspace-1", []domain.DeliveryReceipt{receipt})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Conflict)
	assert.True(t, results[0].Duplicate)
	require.NoError(t, mock.ExpectationsWereMet())
}
