package http

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type browserHTTPDefinitions struct {
	domain.BusinessDefinitionRepository
}

func (browserHTTPDefinitions) Get(context.Context, string, string, string) (*domain.BusinessDefinition, error) {
	return &domain.BusinessDefinition{Kind: "event", Key: "loan.disbursed", Status: "active", Settings: map[string]any{"browser_enabled": true, "authoritative": true, "allowed_browser_origins": []any{"https://shop.example"}}}, nil
}

type browserHTTPWorkspace struct{ domain.WorkspaceRepository }

func (browserHTTPWorkspace) GetByID(context.Context, string) (*domain.Workspace, error) {
	return &domain.Workspace{ID: "w", Settings: domain.WorkspaceSettings{SecretKey: "secret"}}, nil
}
func TestBrowserEventHTTPRequiresServerAuthForCredentialAndRejectsAuthoritative(t *testing.T) {
	svc := service.NewBrowserEventService(nil, browserHTTPDefinitions{}, nil, browserHTTPWorkspace{}, nil)
	handler := NewBrowserEventHandler(svc, func() ([]byte, error) { return []byte("test-secret"), nil })
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	issue := httptest.NewRequest("POST", "/api/browserEvents.issueCredential", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, issue)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("credential issued without auth status=%d", response.Code)
	}
	request := httptest.NewRequest("POST", "/track/events", strings.NewReader(`{"workspace_id":"w","session_id":"session","event":{"event_code":"loan.disbursed","report_id":"r1","data":{}}}`))
	request.Header.Set("Origin", "https://shop.example")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "not open for browser") {
		t.Fatalf("authoritative browser report response=%d %s", response.Code, response.Body.String())
	}
}
