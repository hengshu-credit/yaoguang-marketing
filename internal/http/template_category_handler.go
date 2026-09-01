package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type TemplateCategoryHandler struct {
	service      domain.TemplateCategoryService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewTemplateCategoryHandler(service domain.TemplateCategoryService, getJWTSecret func() ([]byte, error), log logger.Logger) *TemplateCategoryHandler {
	return &TemplateCategoryHandler{service: service, getJWTSecret: getJWTSecret, logger: log}
}

func (h *TemplateCategoryHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	mux.Handle("/api/templateCategories.list", auth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/templateCategories.create", auth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/templateCategories.update", auth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/templateCategories.delete", auth(http.HandlerFunc(h.handleDelete)))
}

func (h *TemplateCategoryHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	includeInactive, _ := strconv.ParseBool(r.URL.Query().Get("include_inactive"))
	categories, err := h.service.List(r.Context(), domain.ListTemplateCategoriesRequest{
		WorkspaceID: r.URL.Query().Get("workspace_id"), IncludeInactive: includeInactive,
	})
	if err != nil {
		h.writeError(w, err, "Failed to list template categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"categories": categories})
}

func (h *TemplateCategoryHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request domain.CreateTemplateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	category, err := h.service.Create(r.Context(), request)
	if err != nil {
		h.writeError(w, err, "Failed to create template category")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"category": category})
}

func (h *TemplateCategoryHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request domain.UpdateTemplateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	category, err := h.service.Update(r.Context(), request)
	if err != nil {
		h.writeError(w, err, "Failed to update template category")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"category": category})
}

func (h *TemplateCategoryHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request domain.DeleteTemplateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.service.Delete(r.Context(), request); err != nil {
		h.writeError(w, err, "Failed to delete template category")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (h *TemplateCategoryHandler) writeError(w http.ResponseWriter, err error, fallback string) {
	var validation domain.ValidationError
	if errors.As(err, &validation) {
		WriteJSONError(w, validation.Error(), http.StatusBadRequest)
		return
	}
	if writeServiceError(w, err, "You do not have access to template categories") {
		return
	}
	if errors.Is(err, domain.ErrTemplateCategoryNotFound) {
		WriteJSONError(w, err.Error(), http.StatusNotFound)
		return
	}
	if errors.Is(err, domain.ErrTemplateCategoryInUse) || errors.Is(err, domain.ErrTemplateCategorySystem) {
		WriteJSONError(w, err.Error(), http.StatusConflict)
		return
	}
	if h.logger != nil {
		h.logger.WithField("error", err.Error()).Error(fallback)
	}
	WriteJSONError(w, fallback, http.StatusInternalServerError)
}

var _ = (*TemplateCategoryHandler)(nil)
