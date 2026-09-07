package http

import (
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"net/http"
)

type ExecutionSnapshotHandler struct {
	service      *service.ExecutionSnapshotService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewExecutionSnapshotHandler(s *service.ExecutionSnapshotService, secret func() ([]byte, error), log logger.Logger) *ExecutionSnapshotHandler {
	return &ExecutionSnapshotHandler{s, secret, log}
}
func (h *ExecutionSnapshotHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/executions.get", auth.RequireAuth()(http.HandlerFunc(h.get)))
	mux.Handle("/api/executions.list", auth.RequireAuth()(http.HandlerFunc(h.list)))
	mux.Handle("/api/executions.observations", auth.RequireAuth()(http.HandlerFunc(h.observations)))
	mux.Handle("/api/executions.asset", auth.RequireAuth()(http.HandlerFunc(h.asset)))
}
func (h *ExecutionSnapshotHandler) read(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return false
	}
	if r.URL.Query().Get("workspace_id") == "" {
		writeAPIError(w, requestIDFor(r), "invalid_request", "workspace_id is required", 400)
		return false
	}
	return true
}
func (h *ExecutionSnapshotHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	code := "execution_error"
	var permission *domain.PermissionError
	if errors.As(err, &permission) {
		status = 403
		code = "permission_denied"
	}
	if errors.Is(err, domain.ErrExecutionSnapshotNotFound) {
		status = 404
		code = "historical_snapshot_missing"
	}
	writeAPIError(w, requestIDFor(r), code, err.Error(), status)
}
func (h *ExecutionSnapshotHandler) get(w http.ResponseWriter, r *http.Request) {
	if !h.read(w, r) {
		return
	}
	q := r.URL.Query()
	result, err := h.service.Get(r.Context(), q.Get("workspace_id"), q.Get("execution_id"), q.Get("after"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}
func (h *ExecutionSnapshotHandler) list(w http.ResponseWriter, r *http.Request) {
	if !h.read(w, r) {
		return
	}
	q := r.URL.Query()
	page, err := h.service.HistoryPage(r.Context(), q.Get("workspace_id"), q.Get("source_id"), q.Get("before"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, 200, page)
}
func (h *ExecutionSnapshotHandler) asset(w http.ResponseWriter, r *http.Request) {
	if !h.read(w, r) {
		return
	}
	q := r.URL.Query()
	asset, err := h.service.Asset(r.Context(), q.Get("workspace_id"), q.Get("execution_id"), q.Get("hash"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", asset.MediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(asset.Data)
}
func (h *ExecutionSnapshotHandler) observations(w http.ResponseWriter, r *http.Request) {
	if !h.read(w, r) {
		return
	}
	q := r.URL.Query()
	result, err := h.service.Observations(r.Context(), q.Get("workspace_id"), q.Get("execution_id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}
