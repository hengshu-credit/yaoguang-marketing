package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	http_handler "github.com/hengshu-credit/yaoguang-marketing/internal/http"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
)

type frequencyPolicyManagerStub struct {
	policies []domain.FrequencyPolicy
}

func (s frequencyPolicyManagerStub) ListFrequencyPolicies(context.Context, string) ([]domain.FrequencyPolicy, error) {
	return s.policies, nil
}
func (s frequencyPolicyManagerStub) SaveFrequencyPolicy(_ context.Context, request domain.SaveFrequencyPolicyRequest) (*domain.FrequencyPolicy, error) {
	return &domain.FrequencyPolicy{ID: "11111111-1111-4111-8111-111111111111", Version: 1, Name: request.Name, Scope: request.Scope}, nil
}

func TestFrequencyPolicyHandlerListsAndSavesVersions(t *testing.T) {
	handler := http_handler.NewFrequencyPolicyHandler(frequencyPolicyManagerStub{policies: []domain.FrequencyPolicy{{ID: "policy-1", Version: 2, Name: "全局频控"}}}, nil, logger.NewLoggerWithLevel("disabled"))
	listRequest := httptest.NewRequest(http.MethodGet, "/api/frequencyPolicies.list?workspace_id=workspace-1", nil)
	listResponse := httptest.NewRecorder()
	handler.HandleList(listResponse, listRequest)
	assert.Equal(t, http.StatusOK, listResponse.Code)
	assert.Contains(t, listResponse.Body.String(), `"version":2`)

	saveRequest := httptest.NewRequest(http.MethodPost, "/api/frequencyPolicies.save", bytes.NewBufferString(`{"workspace_id":"workspace-1","name":"本活动频控","scope":"campaign"}`))
	saveResponse := httptest.NewRecorder()
	handler.HandleSave(saveResponse, saveRequest)
	assert.Equal(t, http.StatusOK, saveResponse.Code)
	assert.Contains(t, saveResponse.Body.String(), `"name":"本活动频控"`)
}
