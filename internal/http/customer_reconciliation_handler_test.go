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

type customerReconciliationHTTPStub struct {
	request *domain.CustomerReconciliationRequest
	get     *domain.CustomerReconciliationGetRequest
	run     *domain.CustomerReconciliationRun
	err     error
}

func (stub *customerReconciliationHTTPStub) Scan(_ context.Context, request *domain.CustomerReconciliationRequest) (*domain.CustomerReconciliationRun, error) {
	stub.request = request
	return stub.run, stub.err
}

func (stub *customerReconciliationHTTPStub) Repair(_ context.Context, request *domain.CustomerReconciliationRequest) (*domain.CustomerReconciliationRun, error) {
	stub.request = request
	return stub.run, stub.err
}

func (stub *customerReconciliationHTTPStub) Get(_ context.Context, request *domain.CustomerReconciliationGetRequest) (*domain.CustomerReconciliationRun, error) {
	stub.get = request
	return stub.run, stub.err
}

func TestCustomerReconciliationHandlerScanReturnsRun(t *testing.T) {
	stub := &customerReconciliationHTTPStub{run: &domain.CustomerReconciliationRun{ID: "run-1", JobType: domain.CustomerReconciliationScan}}
	handler := NewCustomerReconciliationHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodPost, "/api/customers.reconciliation.scan", strings.NewReader(`{"workspace_id":"workspace-1"}`))
	request.Header.Set("X-Request-ID", "request-1")
	response := httptest.NewRecorder()

	handler.handleScan(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.request)
	assert.Equal(t, "workspace-1", stub.request.WorkspaceID)
	var body struct {
		RequestID      string                            `json:"request_id"`
		Reconciliation *domain.CustomerReconciliationRun `json:"reconciliation"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	assert.Equal(t, "request-1", body.RequestID)
	assert.Equal(t, "run-1", body.Reconciliation.ID)
}

func TestCustomerReconciliationHandlerGetParsesWorkspaceAndRun(t *testing.T) {
	runID := "11111111-1111-4111-8111-111111111111"
	stub := &customerReconciliationHTTPStub{run: &domain.CustomerReconciliationRun{ID: runID}}
	handler := NewCustomerReconciliationHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodGet, "/api/customers.reconciliation.get?workspace_id=workspace-1&run_id="+runID, nil)
	response := httptest.NewRecorder()

	handler.handleGet(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.get)
	assert.Equal(t, runID, stub.get.RunID)
}
