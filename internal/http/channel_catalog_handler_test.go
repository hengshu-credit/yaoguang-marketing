package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channelCatalogHTTPStub struct {
	definitions []domain.ChannelDefinition
	err         error
	workspaceID string
}

func (s *channelCatalogHTTPStub) List(_ context.Context, workspaceID string) ([]domain.ChannelDefinition, error) {
	s.workspaceID = workspaceID
	return s.definitions, s.err
}

func TestChannelCatalogHandlerReturnsDefinitionsInServiceOrder(t *testing.T) {
	stub := &channelCatalogHTTPStub{definitions: []domain.ChannelDefinition{
		{ID: "line", LabelKey: "LINE"},
		{ID: "zalo", LabelKey: "Zalo"},
	}}
	handler := NewChannelCatalogHandler(stub, func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/channels.catalog?workspace_id=workspace-1", nil)

	handler.handleList(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "workspace-1", stub.workspaceID)
	var response struct {
		Channels []domain.ChannelDefinition `json:"channels"`
	}
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.Len(t, response.Channels, 2)
	assert.Equal(t, "line", response.Channels[0].ID)
	assert.Equal(t, "zalo", response.Channels[1].ID)
}

func TestChannelCatalogHandlerRejectsNonGetMethod(t *testing.T) {
	handler := NewChannelCatalogHandler(&channelCatalogHTTPStub{}, nil, logger.NewLogger())
	recorder := httptest.NewRecorder()
	handler.handleList(recorder, httptest.NewRequest(http.MethodPost, "/api/channels.catalog", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
}

func TestChannelCatalogHandlerMapsValidationAndAccessErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "validation", err: domain.NewValidationError("workspace_id is required"), want: http.StatusBadRequest},
		{name: "access", err: domain.NewPermissionError(domain.PermissionResourceTemplates, domain.PermissionTypeRead, "denied"), want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewChannelCatalogHandler(&channelCatalogHTTPStub{err: test.err}, nil, logger.NewLogger())
			recorder := httptest.NewRecorder()
			handler.handleList(recorder, httptest.NewRequest(http.MethodGet, "/api/channels.catalog?workspace_id=workspace-1", nil))
			assert.Equal(t, test.want, recorder.Code)
		})
	}
}

func TestChannelCatalogHandlerMapsUnexpectedFailureToServerError(t *testing.T) {
	handler := NewChannelCatalogHandler(&channelCatalogHTTPStub{err: errors.New("database unavailable")}, nil, logger.NewLogger())
	recorder := httptest.NewRecorder()
	handler.handleList(recorder, httptest.NewRequest(http.MethodGet, "/api/channels.catalog?workspace_id=workspace-1", nil))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}
