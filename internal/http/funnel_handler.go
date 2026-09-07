package http

import (
	"encoding/json"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"io"
	"net/http"
)

type FunnelHandler struct {
	service *service.FunnelService
	secret  func() ([]byte, error)
}

func NewFunnelHandler(s *service.FunnelService, secret func() ([]byte, error)) *FunnelHandler {
	return &FunnelHandler{service: s, secret: secret}
}
func (h *FunnelHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.secret)
	mux.Handle("/api/funnels.query", auth.RequireAuth()(http.HandlerFunc(h.query)))
	mux.Handle("/api/funnels.start", auth.RequireAuth()(http.HandlerFunc(h.start)))
	mux.Handle("/api/funnels.get", auth.RequireAuth()(http.HandlerFunc(h.get)))
	mux.Handle("/api/funnels.cancel", auth.RequireAuth()(http.HandlerFunc(h.cancel)))
}

func (h *FunnelHandler) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	var q domain.FunnelQuery
	if !decodeAnalysisRequest(w, r, &q) {
		return
	}
	job, err := h.service.StartQuery(r.Context(), q)
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 202, job)
}
func (h *FunnelHandler) get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	job, err := h.service.GetQuery(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("id"))
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 200, job)
}
func (h *FunnelHandler) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	var q struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
	}
	if !decodeAnalysisRequest(w, r, &q) {
		return
	}
	if err := h.service.CancelQuery(r.Context(), q.WorkspaceID, q.ID); err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"success": true})
}
func (h *FunnelHandler) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	var q domain.FunnelQuery
	if !decodeAnalysisRequest(w, r, &q) {
		return
	}
	result, err := h.service.Query(r.Context(), q)
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}
func decodeAnalysisRequest(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAPIError(w, requestIDFor(r), "validation_error", "Invalid request body", 400)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAPIError(w, requestIDFor(r), "validation_error", "A single JSON object is required", 400)
		return false
	}
	return true
}
func writeAnalysisError(w http.ResponseWriter, r *http.Request, err error) {
	var permission *domain.PermissionError
	var validation domain.ValidationError
	var unauthorized *domain.ErrUnauthorized
	switch {
	case errors.As(err, &permission):
		writeAPIError(w, requestIDFor(r), "permission_denied", err.Error(), 403)
	case errors.As(err, &unauthorized):
		writeAPIError(w, requestIDFor(r), "unauthorized", "Authentication required", 401)
	case errors.As(err, &validation):
		writeAPIError(w, requestIDFor(r), "validation_error", err.Error(), 400)
	case errors.Is(err, domain.ErrBusinessDefinitionNotFound):
		writeAPIError(w, requestIDFor(r), "not_found", "Definition not found", 404)
	default:
		writeAPIError(w, requestIDFor(r), "analysis_failed", "Unable to complete analysis", 500)
	}
}
