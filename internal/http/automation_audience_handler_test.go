package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type automationAudienceRunnerStub struct {
	request service.AutomationAudienceRunRequest
}

func (s *automationAudienceRunnerStub) Start(_ context.Context, request service.AutomationAudienceRunRequest) (*service.AutomationAudienceRunResult, error) {
	s.request = request
	return &service.AutomationAudienceRunResult{
		AutomationID: request.AutomationID, AudienceID: request.AudienceID, AudienceVersion: 7,
		BuildID: "build-7", CandidateCount: 3, EnrolledCount: 2,
	}, nil
}

func TestAutomationHandlerStartAudienceReturnsResolvedRun(t *testing.T) {
	runner := &automationAudienceRunnerStub{}
	handler := &AutomationHandler{audienceRuns: runner}
	request := httptest.NewRequest(http.MethodPost, "/api/automations.startAudience", strings.NewReader(`{
		"workspace_id":"workspace-1","automation_id":"automation-1","audience_id":"audience-1"
	}`))
	response := httptest.NewRecorder()

	handler.handleStartAudience(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "workspace-1", runner.request.WorkspaceID)
	assert.Contains(t, response.Body.String(), `"audience_version":7`)
	assert.Contains(t, response.Body.String(), `"enrolled_count":2`)
}
