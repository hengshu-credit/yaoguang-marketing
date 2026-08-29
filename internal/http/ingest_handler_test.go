package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ingestServiceHTTPStub struct {
	request  *domain.IngestBatchRequest
	response *domain.IngestBatchResponse
	err      error
}

func (s *ingestServiceHTTPStub) IngestBatch(_ context.Context, request *domain.IngestBatchRequest) (*domain.IngestBatchResponse, error) {
	s.request = request
	return s.response, s.err
}

func TestIngestHandlerBatchReturnsPerItemResponse(t *testing.T) {
	stub := &ingestServiceHTTPStub{response: &domain.IngestBatchResponse{
		Accepted: 1,
		Results:  []domain.IngestItemResult{{ID: "contact-1", Type: "contact", Status: "accepted"}},
	}}
	handler := NewIngestHandler(stub, func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger())
	body := `{"workspace_id":"workspace-1","contacts":[{"id":"contact-1","contact":{"email":"user@example.com"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest.batch", strings.NewReader(body))
	response := httptest.NewRecorder()

	handler.handleBatch(response, req)

	assert.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.request)
	assert.Equal(t, "workspace-1", stub.request.WorkspaceID)
	var decoded domain.IngestBatchResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&decoded))
	assert.Equal(t, 1, decoded.Accepted)
}

func TestIngestHandlerBatchReturns429WithoutQueueing(t *testing.T) {
	stub := &ingestServiceHTTPStub{err: service.ErrIngestBusy}
	handler := NewIngestHandler(stub, nil, logger.NewLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/ingest.batch", bytes.NewBufferString(`{"workspace_id":"workspace-1","events":[{"id":"e","email":"u@example.com","event_name":"x","external_id":"1"}]}`))
	response := httptest.NewRecorder()

	handler.handleBatch(response, req)

	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.Equal(t, "1", response.Header().Get("Retry-After"))
}

func TestIngestHandlerBatchRejectsOversizedBody(t *testing.T) {
	handler := NewIngestHandler(&ingestServiceHTTPStub{}, nil, logger.NewLogger())
	oversized := `{"workspace_id":"workspace-1","contacts":[],"padding":"` + strings.Repeat("x", int(ingestMaxBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest.batch", strings.NewReader(oversized))
	response := httptest.NewRecorder()

	handler.handleBatch(response, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}

func TestIngestHandlerBatchMapsValidationErrors(t *testing.T) {
	stub := &ingestServiceHTTPStub{err: domain.NewValidationError("batch is invalid")}
	handler := NewIngestHandler(stub, nil, logger.NewLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/ingest.batch", strings.NewReader(`{"workspace_id":"workspace-1","contacts":[]}`))
	response := httptest.NewRecorder()

	handler.handleBatch(response, req)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.False(t, errors.Is(stub.err, service.ErrIngestBusy))
}

func TestIngestHandlerBatchRejectsTrailingJSON(t *testing.T) {
	stub := &ingestServiceHTTPStub{}
	handler := NewIngestHandler(stub, nil, logger.NewLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/ingest.batch", strings.NewReader(
		`{"workspace_id":"workspace-1","events":[{"id":"e","email":"u@example.com","event_name":"x","external_id":"1"}]} {}`,
	))
	response := httptest.NewRecorder()

	handler.handleBatch(response, req)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Nil(t, stub.request)
}
