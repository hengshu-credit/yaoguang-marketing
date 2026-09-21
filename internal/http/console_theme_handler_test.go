package http

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type httpThemeStore struct{ writes int }

func (s *httpThemeStore) Set(context.Context, string, *domain.ConsoleThemeSettings) error {
	s.writes++
	return nil
}
func TestConsoleThemeHandlerValidation(t *testing.T) {
	store := &httpThemeStore{}
	h := NewConsoleThemeHandler(service.NewConsoleThemeService(store, reportingConfigAuth{}))
	for _, body := range []string{`{}`, `{"workspace_id":"w","console_theme":{"primary_color":"red"}}`, `{"workspace_id":"w","console_theme":{"injected_css":"x"}}`, `{"workspace_id":"w","console_theme":{}} {}`} {
		w := httptest.NewRecorder()
		h.save(w, httptest.NewRequest(http.MethodPost, "/api/workspaces.setConsoleTheme", strings.NewReader(body)))
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	require.Zero(t, store.writes)
	w := httptest.NewRecorder()
	h.save(w, httptest.NewRequest(http.MethodPost, "/api/workspaces.setConsoleTheme", strings.NewReader(`{"workspace_id":"w","console_theme":{}}`)))
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"accent_preset":"yaoguang"`)
	require.Equal(t, 1, store.writes)
}
