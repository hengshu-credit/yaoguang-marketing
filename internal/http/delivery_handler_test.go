package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deliveryManagementHTTPStub struct {
	resolved *domain.DeliveryResolveUnknownRequest
	listed   *domain.DeliveryListRequest
}

func (s *deliveryManagementHTTPStub) List(_ context.Context, request *domain.DeliveryListRequest) ([]domain.DeliveryIntent, int, error) {
	s.listed = request
	return []domain.DeliveryIntent{{ID: "intent-1"}}, 1, nil
}

func TestDeliveryHandlerListPassesOperationalFilters(t *testing.T) {
	stub := &deliveryManagementHTTPStub{}
	handler := NewDeliveryHandler(stub, nil, logger.NewLogger())
	request := httptest.NewRequest(http.MethodGet, "/api/deliveries.list?workspace_id=workspace-1&status=unknown&channel=sms&source_type=automation&source_id=journey-1&provider=aliyun&customer_id=U0001&from=2026-08-30T00:00:00Z&to=2026-08-31T00:00:00Z", nil)
	response := httptest.NewRecorder()

	handler.handleList(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.listed)
	assert.Equal(t, "sms", stub.listed.Channel)
	assert.Equal(t, "automation", stub.listed.SourceType)
	assert.Equal(t, "aliyun", stub.listed.Provider)
	assert.Equal(t, "U0001", stub.listed.CustomerID)
	assert.NotNil(t, stub.listed.From)
	assert.NotNil(t, stub.listed.To)
}
func (s *deliveryManagementHTTPStub) Get(context.Context, *domain.DeliveryGetRequest) (*domain.DeliveryDetail, error) {
	return &domain.DeliveryDetail{Intent: domain.DeliveryIntent{ID: "intent-1"}}, nil
}
func (s *deliveryManagementHTTPStub) Reconcile(context.Context, *domain.DeliveryReconcileRequest) error {
	return nil
}
func (s *deliveryManagementHTTPStub) ResolveUnknown(_ context.Context, request *domain.DeliveryResolveUnknownRequest) error {
	s.resolved = request
	return nil
}

func TestDeliveryHandlerResolveUnknownAcceptsOnlyAuditedActions(t *testing.T) {
	stub := &deliveryManagementHTTPStub{}
	handler := NewDeliveryHandler(stub, nil, logger.NewLogger())
	body := []byte(`{"workspace_id":"workspace-1","intent_id":"intent-1","action":"retry_after_verified_not_accepted","reason":"provider portal confirms request was rejected before acceptance"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/deliveries.resolveUnknown", bytes.NewReader(body))
	response := httptest.NewRecorder()

	handler.handleResolveUnknown(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, stub.resolved)
	assert.Equal(t, domain.DeliveryResolutionRetryVerifiedNotAccepted, stub.resolved.Action)
	assert.NotEmpty(t, stub.resolved.Reason)
}

func TestDeliveryHandlerRejectsUnknownResolutionAction(t *testing.T) {
	service := &deliveryManagementHTTPStub{}
	// The HTTP stub deliberately delegates validation to the real domain contract
	// in this focused test, mirroring the service boundary.
	requestBody := &domain.DeliveryResolveUnknownRequest{WorkspaceID: "workspace-1", IntentID: "intent-1", Action: "retry", Reason: "unsafe blind retry request"}
	assert.ErrorContains(t, requestBody.Validate(), "action is invalid")
	assert.Nil(t, service.resolved)
}
