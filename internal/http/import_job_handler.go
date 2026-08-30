package http

import (
	"net/http"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type ImportJobHandler struct {
	service      *service.ImportJobService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewImportJobHandler(service *service.ImportJobService, getJWTSecret func() ([]byte, error), log logger.Logger) *ImportJobHandler {
	return &ImportJobHandler{service: service, getJWTSecret: getJWTSecret, logger: log}
}

func (h *ImportJobHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/imports.upload", auth.RequireAuth()(http.HandlerFunc(h.upload)))
	mux.Handle("/api/imports.get", auth.RequireAuth()(http.HandlerFunc(h.get)))
	mux.Handle("/api/imports.process", auth.RequireAuth()(http.HandlerFunc(h.process)))
}

func (h *ImportJobHandler) upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	if filename == "" {
		filename = "customers.csv"
	}
	job, err := h.service.StageCSV(r.Context(), workspaceID, filename, r.Body)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (h *ImportJobHandler) get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job, err := h.service.Get(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("job_id"))
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *ImportJobHandler) process(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	processed, err := h.service.ProcessNextChunk(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("job_id"))
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"processed": processed})
}

func (h *ImportJobHandler) error(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.WithField("error", err.Error()).Error("Import job request failed")
	status := http.StatusBadRequest
	if _, ok := err.(*domain.PermissionError); ok {
		status = http.StatusForbidden
	}
	writeAPIError(w, requestIDFor(r), "import_error", err.Error(), status)
}
