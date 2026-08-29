package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryReceiptValidateAndPayloadHash(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 29, 8, 30, 0, 0, time.UTC)
	receipt := DeliveryReceipt{
		Provider:          DeliveryProviderTwilio,
		ReceiptID:         "receipt-1",
		ProviderMessageID: "SM11111111111111111111111111111111",
		Event:             DeliveryReceiptDelivered,
		OccurredAt:        occurredAt,
		Metadata: map[string]interface{}{
			"b": float64(2),
			"a": "one",
		},
	}
	require.NoError(t, receipt.Validate())

	hash1, err := receipt.ComputePayloadHash()
	require.NoError(t, err)
	receipt.ReceivedAt = occurredAt.Add(time.Hour)
	receipt.Metadata = map[string]interface{}{"a": "one", "b": float64(2)}
	hash2, err := receipt.ComputePayloadHash()
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2, "server receipt time and map order must not alter idempotency")

	receipt.Event = DeliveryReceiptFailed
	hash3, err := receipt.ComputePayloadHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestDeliveryReceiptValidateRejectsInvalidInput(t *testing.T) {
	valid := DeliveryReceipt{
		Provider:          DeliveryProviderFCM,
		ReceiptID:         "receipt-1",
		ProviderMessageID: "projects/project/messages/message-1",
		Event:             DeliveryReceiptAccepted,
		OccurredAt:        time.Now().UTC(),
	}

	tests := []struct {
		name   string
		mutate func(*DeliveryReceipt)
		want   string
	}{
		{"provider", func(r *DeliveryReceipt) { r.Provider = "unknown" }, "provider"},
		{"receipt id", func(r *DeliveryReceipt) { r.ReceiptID = "" }, "receipt_id"},
		{"event", func(r *DeliveryReceipt) { r.Event = "unknown" }, "event"},
		{"occurred at", func(r *DeliveryReceipt) { r.OccurredAt = time.Time{} }, "occurred_at"},
		{"message identity", func(r *DeliveryReceipt) { r.ProviderMessageID = "" }, "provider_message_id or message_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := valid
			test.mutate(&receipt)
			err := receipt.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestIngestDeliveryReceiptsRequestValidate(t *testing.T) {
	request := IngestDeliveryReceiptsRequest{WorkspaceID: "workspace-1", Receipts: []DeliveryReceipt{{}}}
	require.NoError(t, request.ValidateEnvelope(500))

	request.WorkspaceID = ""
	assert.ErrorContains(t, request.ValidateEnvelope(500), "workspace_id")
	request.WorkspaceID = "workspace-1"
	request.Receipts = nil
	assert.ErrorContains(t, request.ValidateEnvelope(500), "at least one")
	request.Receipts = make([]DeliveryReceipt, 501)
	assert.ErrorContains(t, request.ValidateEnvelope(500), "at most 500")
}
