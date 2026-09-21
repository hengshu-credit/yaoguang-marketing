package http

import (
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"io"
	"net/http"
)

type ConsoleThemeHandler struct{ service *service.ConsoleThemeService }

func NewConsoleThemeHandler(s *service.ConsoleThemeService) *ConsoleThemeHandler {
	return &ConsoleThemeHandler{service: s}
}
func (h *ConsoleThemeHandler) RegisterRoutes(mux *http.ServeMux, secret func() ([]byte, error), isDemo bool) {
	mux.Handle("/api/workspaces.setConsoleTheme", middleware.RestrictedInDemo(isDemo)(middleware.NewAuthMiddleware(secret).RequireAuth()(http.HandlerFunc(h.save))))
}
func (h *ConsoleThemeHandler) save(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", 405)
		return
	}
	var request struct {
		WorkspaceID string                       `json:"workspace_id"`
		Theme       *domain.ConsoleThemeSettings `json:"console_theme"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		WriteJSONError(w, "Invalid console theme request", 400)
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		WriteJSONError(w, "Invalid console theme request", 400)
		return
	}
	value, err := h.service.Save(r.Context(), request.WorkspaceID, request.Theme)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"console_theme": value})
}
