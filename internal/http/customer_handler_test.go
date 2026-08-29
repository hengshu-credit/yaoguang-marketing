package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customerServiceHTTPStub struct {
	getRequest     *domain.GetCustomerRequest
	upsertRequest  *domain.UpsertCustomerRequest
	batchRequest   *domain.CustomerBatchUpsertRequest
	mergeRequest   *domain.CustomerMergeRequest
	getResponse    *domain.Customer
	upsertResponse *domain.CustomerMutationResult
	batchResponse  *domain.CustomerBatchUpsertResponse
	mergeResponse  *domain.CustomerMergeResult
	err            error
}

func (stub *customerServiceHTTPStub) GetCustomer(_ context.Context, request *domain.GetCustomerRequest) (*domain.Customer, error) {
	stub.getRequest = request
	return stub.getResponse, stub.err
}

func (stub *customerServiceHTTPStub) UpsertCustomer(_ context.Context, request *domain.UpsertCustomerRequest) (*domain.CustomerMutationResult, error) {
	stub.upsertRequest = request
	return stub.upsertResponse, stub.err
}

func (stub *customerServiceHTTPStub) UpsertCustomerBatch(_ context.Context, request *domain.CustomerBatchUpsertRequest) (*domain.CustomerBatchUpsertResponse, error) {
	stub.batchRequest = request
	return stub.batchResponse, stub.err
}

func (stub *customerServiceHTTPStub) MergeCustomer(_ context.Context, request *domain.CustomerMergeRequest) (*domain.CustomerMergeResult, error) {
	stub.mergeRequest = request
	return stub.mergeResponse, stub.err
}

func TestCustomerHandlerUpsertReturnsRequestIDAndMutation(t *testing.T) {
	stub := &customerServiceHTTPStub{upsertResponse: &domain.CustomerMutationResult{CustomerID: "customer-1", Action: "created"}}
	handler := NewCustomerHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/customers.upsert", strings.NewReader(`{
		"workspace_id":"workspace1","idempotency_key":"idem-1","customer":{"external_user_id":"external-1"}
	}`))
	request.Header.Set("X-Request-ID", "request-1")
	response := httptest.NewRecorder()

	handler.handleUpsert(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "request-1", response.Header().Get("X-Request-ID"))
	require.NotNil(t, stub.upsertRequest)
	var body struct {
		RequestID string                         `json:"request_id"`
		Customer  *domain.CustomerMutationResult `json:"customer"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	assert.Equal(t, "request-1", body.RequestID)
	assert.Equal(t, "customer-1", body.Customer.CustomerID)
}

func TestCustomerHandlerGetNeverEchoesRawIdentityValue(t *testing.T) {
	stub := &customerServiceHTTPStub{getResponse: &domain.Customer{
		ID: "customer-1", Identities: []domain.CustomerIdentity{{Type: domain.CustomerIdentityEmail, DisplayHint: "a***@example.com"}},
	}}
	handler := NewCustomerHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/customers.get", strings.NewReader(`{
		"workspace_id":"workspace1","locator":{"identity":{"type":"email","value":"alice@example.com"}}
	}`))
	response := httptest.NewRecorder()

	handler.handleGet(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), "alice@example.com")
	assert.Contains(t, response.Body.String(), "a***@example.com")
}

func TestCustomerHandlerMapsTypedConflictsAndValidation(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "identity", err: &domain.ErrCustomerIdentityConflict{IdentityType: domain.CustomerIdentityEmail}, wantStatus: http.StatusConflict, wantCode: "identity_conflict"},
		{name: "idempotency", err: &domain.ErrCustomerIdempotencyConflict{}, wantStatus: http.StatusConflict, wantCode: "idempotency_conflict"},
		{name: "not found", err: &domain.ErrCustomerNotFound{}, wantStatus: http.StatusNotFound, wantCode: "customer_not_found"},
		{name: "validation", err: domain.NewValidationError("bad request"), wantStatus: http.StatusBadRequest, wantCode: "validation_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &customerServiceHTTPStub{err: tt.err}
			handler := NewCustomerHandler(stub, nil, logger.NewLogger())
			request := httptest.NewRequest(http.MethodPost, "/api/customers.get", strings.NewReader(`{"workspace_id":"workspace1","locator":{"external_user_id":"external-1"}}`))
			response := httptest.NewRecorder()
			handler.handleGet(response, request)

			assert.Equal(t, tt.wantStatus, response.Code)
			assert.Contains(t, response.Body.String(), `"code":"`+tt.wantCode+`"`)
			assert.Contains(t, response.Body.String(), `"request_id":`)
		})
	}
}

func TestCustomerHandlerBatchPreservesCompleteOrderedResults(t *testing.T) {
	stub := &customerServiceHTTPStub{batchResponse: &domain.CustomerBatchUpsertResponse{
		Accepted: 1, Failed: 1,
		Results: []domain.CustomerBatchItemResult{
			{Index: 0, Status: "accepted", Customer: &domain.CustomerMutationResult{CustomerID: "customer-1"}},
			{Index: 1, Status: "error", Error: &domain.CustomerBatchItemError{Code: "validation_error", Message: "invalid"}},
		},
	}}
	handler := NewCustomerHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/customers.batch", strings.NewReader(`{
		"workspace_id":"workspace1","items":[{"idempotency_key":"one","customer":{"external_user_id":"one"}}]
	}`))
	response := httptest.NewRecorder()
	handler.handleBatch(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	var body struct {
		RequestID string                           `json:"request_id"`
		Accepted  int                              `json:"accepted"`
		Failed    int                              `json:"failed"`
		Results   []domain.CustomerBatchItemResult `json:"results"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	assert.Equal(t, 1, body.Accepted)
	assert.Equal(t, 1, body.Failed)
	assert.Equal(t, []int{0, 1}, []int{body.Results[0].Index, body.Results[1].Index})
}

func TestCustomerHandlerRejectsWrongMethodMalformedAndTrailingJSON(t *testing.T) {
	handler := NewCustomerHandler(&customerServiceHTTPStub{}, nil, logger.NewLogger())

	wrongMethod := httptest.NewRecorder()
	handler.handleMerge(wrongMethod, httptest.NewRequest(http.MethodGet, "/api/customers.merge", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, wrongMethod.Code)

	malformed := httptest.NewRecorder()
	handler.handleMerge(malformed, httptest.NewRequest(http.MethodPost, "/api/customers.merge", strings.NewReader(`{"workspace_id":`)))
	assert.Equal(t, http.StatusBadRequest, malformed.Code)

	trailing := httptest.NewRecorder()
	handler.handleMerge(trailing, httptest.NewRequest(http.MethodPost, "/api/customers.merge", strings.NewReader(`{} {}`)))
	assert.Equal(t, http.StatusBadRequest, trailing.Code)
}

func TestCustomerRoutesReturnStructuredRequestIDForAuthenticationFailures(t *testing.T) {
	handler := NewCustomerHandler(
		&customerServiceHTTPStub{},
		func() ([]byte, error) { return []byte("customer-handler-test-jwt-secret-32-bytes"), nil },
		logger.NewLogger(),
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/customers.get", strings.NewReader(`{}`))
	request.Header.Set("X-Request-ID", "auth-request-1")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Equal(t, "auth-request-1", response.Header().Get("X-Request-ID"))
	assert.JSONEq(t, `{
		"request_id":"auth-request-1",
		"error":{"code":"unauthorized","message":"Authorization header is required"}
	}`, response.Body.String())
}
