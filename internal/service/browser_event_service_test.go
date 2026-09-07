package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"testing"
	"time"
)

type browserTestWorkspace struct {
	domain.WorkspaceRepository
	workspace *domain.Workspace
}

func (r browserTestWorkspace) GetByID(context.Context, string) (*domain.Workspace, error) {
	return r.workspace, nil
}
func TestBrowserEventRejectsAuthoritativeAndUnlistedOrigin(t *testing.T) {
	base := &domain.BusinessDefinition{Status: "active", Settings: map[string]any{"browser_enabled": true, "allowed_browser_origins": []any{"https://shop.example"}}}
	if err := validateBrowserEventDefinition(base, "https://shop.example"); err != nil {
		t.Fatal(err)
	}
	if validateBrowserEventDefinition(base, "https://evil.example") == nil {
		t.Fatal("unlisted origin accepted")
	}
	base.Settings["authoritative"] = true
	if validateBrowserEventDefinition(base, "https://shop.example") == nil {
		t.Fatal("authoritative browser report accepted")
	}
	delete(base.Settings, "authoritative")
	base.Settings["builtin"] = true
	if validateBrowserEventDefinition(base, "https://shop.example") == nil {
		t.Fatal("builtin browser report accepted")
	}
}
func TestBrowserEventRejectsForgedIdentityAndTouchpointToken(t *testing.T) {
	defs := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{"event:login": {Kind: "event", Key: "login", Status: "active", Settings: map[string]any{"browser_enabled": true, "allowed_browser_origins": []any{"https://shop.example"}}}}}
	s := NewBrowserEventService(nil, defs, nil, browserTestWorkspace{workspace: &domain.Workspace{Settings: domain.WorkspaceSettings{SecretKey: "secret"}}}, nil)
	_, err := s.Track(context.Background(), domain.BrowserTrackEventRequest{WorkspaceID: "w", SessionID: "s", Event: map[string]any{"event_code": "login", "customer_attributes": map[string]any{"customer_id": "forged"}}}, "https://shop.example")
	if err == nil {
		t.Fatal("unsigned customer accepted")
	}
	touchpoint, err := domain.BuildWebIdentifyToken("person@example.com", "secret", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Track(context.Background(), domain.BrowserTrackEventRequest{WorkspaceID: "w", SessionID: "s", Credential: touchpoint, Event: map[string]any{"event_code": "login"}}, "https://shop.example")
	if err == nil {
		t.Fatal("forwardable touchpoint token accepted as identity")
	}
}
