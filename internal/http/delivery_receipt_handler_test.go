package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/ratelimiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deliveryReceiptHTTPStub struct {
	ingestRequest  *domain.IngestDeliveryReceiptsRequest
	ingestResult   *domain.IngestDeliveryReceiptsResponse
	ingestErr      error
	callback       domain.TwilioDeliveryCallback
	callbackResult *domain.DeliveryReceiptRecordResult
	callbackErr    error
}

func (s *deliveryReceiptHTTPStub) Ingest(_ context.Context, request *domain.IngestDeliveryReceiptsRequest) (*domain.IngestDeliveryReceiptsResponse, error) {
	s.ingestRequest = request
	return s.ingestResult, s.ingestErr
}

func (s *deliveryReceiptHTTPStub) ProcessTwilioCallback(_ context.Context, callback domain.TwilioDeliveryCallback) (*domain.DeliveryReceiptRecordResult, error) {
	s.callback = callback
	return s.callbackResult, s.callbackErr
}

func TestDeliveryReceiptHandlerIngest(t *testing.T) {
	stub := &deliveryReceiptHTTPStub{ingestResult: &domain.IngestDeliveryReceiptsResponse{Accepted: 1}}
	handler := NewDeliveryReceiptHandler(stub, func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger(), "https://notify.example")
	request := httptest.NewRequest(http.MethodPost, "/api/deliveryReceipts.ingest", strings.NewReader(
		`{"workspace_id":"workspace-1","receipts":[{"provider":"fcm","receipt_id":"r1","provider_message_id":"m1","event":"accepted","occurred_at":"2026-08-29T08:30:00Z"}]}`,
	))
	response := httptest.NewRecorder()
	handler.handleIngest(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.ingestRequest)
	assert.Equal(t, "workspace-1", stub.ingestRequest.WorkspaceID)
	var decoded domain.IngestDeliveryReceiptsResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&decoded))
	assert.Equal(t, 1, decoded.Accepted)
}

func TestDeliveryReceiptHandlerBuildsCanonicalTwilioCallbackURL(t *testing.T) {
	stub := &deliveryReceiptHTTPStub{callbackResult: &domain.DeliveryReceiptRecordResult{ReceiptID: "r1", Applied: true}}
	handler := NewDeliveryReceiptHandler(stub, nil, logger.NewLogger(), "https://notify.example/")
	request := httptest.NewRequest(
		http.MethodPost,
		"/webhooks/delivery/twilio?workspace_id=workspace-1&integration_id=sms-1&message_id=message-1&effect_key=effect-1",
		strings.NewReader("MessageSid=SM111&MessageStatus=delivered"),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Twilio-Signature", "signature")
	response := httptest.NewRecorder()
	handler.handleTwilio(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "https://notify.example"+request.URL.RequestURI(), stub.callback.CallbackURL)
	assert.Equal(t, "SM111", stub.callback.Form["MessageSid"][0])
	assert.Equal(t, "message-1", stub.callback.MessageID)
	assert.Equal(t, "effect-1", stub.callback.EffectKey)
}

func TestDeliveryReceiptHandlerRejectsInvalidTwilioSignature(t *testing.T) {
	stub := &deliveryReceiptHTTPStub{callbackErr: service.ErrInvalidTwilioSignature}
	handler := NewDeliveryReceiptHandler(stub, nil, logger.NewLogger(), "https://notify.example")
	request := httptest.NewRequest(http.MethodPost, "/webhooks/delivery/twilio?workspace_id=workspace-1&integration_id=sms-1", strings.NewReader("MessageSid=SM111&MessageStatus=delivered"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Twilio-Signature", "bad")
	response := httptest.NewRecorder()
	handler.handleTwilio(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestDeliveryReceiptHandlerRateLimitsTwilioBeforeService(t *testing.T) {
	stub := &deliveryReceiptHTTPStub{callbackResult: &domain.DeliveryReceiptRecordResult{ReceiptID: "r1"}}
	limiter := ratelimiter.NewRateLimiter()
	t.Cleanup(limiter.Stop)
	limiter.SetPolicy("delivery_receipt:twilio:ip", 1, time.Minute)
	limiter.SetPolicy("delivery_receipt:twilio:workspace", 10, time.Minute)
	handler := NewDeliveryReceiptHandler(stub, nil, logger.NewLogger(), "https://notify.example", limiter)

	makeRequest := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/webhooks/delivery/twilio?workspace_id=workspace-1&integration_id=sms-1", strings.NewReader("MessageSid=SM111&MessageStatus=delivered"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("X-Twilio-Signature", "signature")
		response := httptest.NewRecorder()
		handler.handleTwilio(response, request)
		return response
	}
	assert.Equal(t, http.StatusOK, makeRequest().Code)
	assert.Equal(t, http.StatusTooManyRequests, makeRequest().Code)
}
