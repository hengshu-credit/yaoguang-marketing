package http

import (
	"encoding/csv"
	"net/http"
	"strconv"
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
	mux.Handle("/api/imports.list", auth.RequireAuth()(http.HandlerFunc(h.list)))
	mux.Handle("/api/imports.cancel", auth.RequireAuth()(http.HandlerFunc(h.cancel)))
	mux.Handle("/api/imports.errors", auth.RequireAuth()(http.HandlerFunc(h.errors)))
	mux.Handle("/api/imports.process", auth.RequireAuth()(http.HandlerFunc(h.process)))
}

func (h *ImportJobHandler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	result, err := h.service.List(r.Context(), r.URL.Query().Get("workspace_id"), limit, offset)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ImportJobHandler) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.service.Cancel(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("job_id")); err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}

func (h *ImportJobHandler) errors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.Errors(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("job_id"), limit, offset)
	if err != nil {
		h.error(w, r, err)
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="import-errors.csv"`)
		_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"行号", "外部用户ID", "脱敏联系方式", "错误码", "错误说明"})
		for _, item := range items {
			_ = writer.Write([]string{strconv.FormatInt(item.Ordinal, 10), item.ExternalUserID, item.DisplayIdentity, item.ErrorCode, item.ErrorDetail})
		}
		writer.Flush()
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": total, "limit": limit, "offset": offset})
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
	job, err := h.service.StageCSV(r.Context(), workspaceID, filename, r.URL.Query()["list_id"], r.Body)
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
