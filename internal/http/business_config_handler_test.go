package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/require"
)

type reportingConfigAuth struct{ domain.AuthService }

func (reportingConfigAuth) AuthenticateUserForWorkspace(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	return ctx, &domain.User{}, &domain.UserWorkspace{Role: "owner"}, nil
}

type reportingConfigRepository struct {
	domain.BusinessDefinitionRepository
	writes int
}

func (*reportingConfigRepository) Get(context.Context, string, string, string) (*domain.BusinessDefinition, error) {
	return nil, domain.ErrBusinessDefinitionNotFound
}
func (r *reportingConfigRepository) Save(_ context.Context, _ string, d *domain.BusinessDefinition) (*domain.BusinessDefinition, error) {
	r.writes++
	return d, nil
}

func TestEventReportingHTTPConfigurationAndPreview(t *testing.T) {
	repo := &reportingConfigRepository{}
	handler := NewBusinessConfigHandler(service.NewBusinessConfigService(repo, reportingConfigAuth{}), nil)
	call := func(action, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.handle(action)(response, httptest.NewRequest(http.MethodPost, "/api/businessDefinitions."+action, strings.NewReader(body)))
		return response
	}
	response := call("save", `{"workspace_id":"ws","definition":{"kind":"event","key":"login","name":"Login","status":"active","settings":{"browser_enabled":true,"authoritative":true}}}`)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), `"field":"settings.browser_enabled"`)
	require.Zero(t, repo.writes)
	response = call("save", `{"workspace_id":"ws","definition":{"kind":"event","key":"login","name":"Login","status":"draft","settings":{"expiry_mode":"duration"}}}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, 1, repo.writes)
	response = call("preview", `{"workspace_id":"ws","definition":{"kind":"event","key":"login","name":"Login","status":"draft","settings":{"expiry_mode":"duration"}},"sample":{}}`)
	require.Equal(t, http.StatusOK, response.Code)
	var result service.BusinessPreviewResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.False(t, result.Valid)
	require.Equal(t, "settings.ttl_seconds", result.Errors[0].Field)
	require.Equal(t, 1, repo.writes, "preview must not write")
}
