package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contactEndpointServiceHTTPStub struct {
	request   *domain.ListContactEndpointsRequest
	endpoints []*domain.ContactEndpoint
	err       error
}

func (s *contactEndpointServiceHTTPStub) List(
	_ context.Context, request *domain.ListContactEndpointsRequest,
) ([]*domain.ContactEndpoint, error) {
	s.request = request
	return s.endpoints, s.err
}

func TestContactEndpointHandlerListReturnsMetadataWithoutAddress(t *testing.T) {
	stub := &contactEndpointServiceHTTPStub{endpoints: []*domain.ContactEndpoint{{
		EndpointID: "device-1", Email: "user@example.com", Channel: domain.ChannelPush,
		Provider: domain.PushProviderFCM, Platform: domain.EndpointPlatformAndroid,
		Address: "secret-token", Enabled: true,
	}}}
	handler := NewContactEndpointHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodGet,
		"/api/contactEndpoints.list?workspace_id=workspace-1&email=user%40example.com", nil)
	response := httptest.NewRecorder()

	handler.handleList(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.request)
	assert.Equal(t, "workspace-1", stub.request.WorkspaceID)
	assert.NotContains(t, response.Body.String(), "secret-token")
	assert.NotContains(t, response.Body.String(), "address")
}

func TestContactEndpointHandlerListValidatesQuery(t *testing.T) {
	handler := NewContactEndpointHandler(&contactEndpointServiceHTTPStub{}, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodGet,
		"/api/contactEndpoints.list?workspace_id=workspace-1&email=invalid", nil)
	response := httptest.NewRecorder()

	handler.handleList(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}
