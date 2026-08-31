package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type CampaignHandler struct {
	service      *service.CampaignService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewCampaignHandler(service *service.CampaignService, getJWTSecret func() ([]byte, error), log logger.Logger) *CampaignHandler {
	return &CampaignHandler{service: service, getJWTSecret: getJWTSecret, logger: log}
}

func (h *CampaignHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/campaigns.create", auth.RequireAuth()(http.HandlerFunc(h.create)))
	mux.Handle("/api/campaigns.list", auth.RequireAuth()(http.HandlerFunc(h.list)))
	mux.Handle("/api/campaigns.start", auth.RequireAuth()(http.HandlerFunc(h.start)))
	mux.Handle("/api/campaigns.run", auth.RequireAuth()(http.HandlerFunc(h.run)))
	mux.Handle("/api/campaigns.recipients", auth.RequireAuth()(http.HandlerFunc(h.recipients)))
}

func (h *CampaignHandler) create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request := struct {
		WorkspaceID     string                   `json:"workspace_id"`
		Name            string                   `json:"name"`
		AudienceID      string                   `json:"audience_id"`
		AudienceVersion int                      `json:"audience_version"`
		ListID          string                   `json:"list_id"`
		Channel         string                   `json:"channel"`
		Variants        []domain.CampaignVariant `json:"variants"`
	}{}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeAPIError(w, requestIDFor(r), "invalid_request", err.Error(), http.StatusBadRequest)
		return
	}
	item, err := h.service.Create(r.Context(), service.CreateCampaignRequest{WorkspaceID: request.WorkspaceID, Name: request.Name,
		AudienceID: request.AudienceID, AudienceVersion: request.AudienceVersion, ListID: request.ListID, Channel: request.Channel, Variants: request.Variants})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *CampaignHandler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.List(r.Context(), r.URL.Query().Get("workspace_id"), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": total})
}

func (h *CampaignHandler) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request := struct {
		WorkspaceID string `json:"workspace_id"`
		CampaignID  string `json:"campaign_id"`
		Version     int    `json:"version"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAPIError(w, requestIDFor(r), "invalid_request", err.Error(), http.StatusBadRequest)
		return
	}
	run, err := h.service.Start(r.Context(), request.WorkspaceID, request.CampaignID, request.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (h *CampaignHandler) run(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	item, err := h.service.GetRun(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("run_id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *CampaignHandler) recipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, next, err := h.service.Recipients(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("run_id"), after, limit)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "next": next})
}

func (h *CampaignHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.WithField("error", err.Error()).Error("Campaign request failed")
	status := http.StatusBadRequest
	if _, ok := err.(*domain.PermissionError); ok {
		status = http.StatusForbidden
	}
	writeAPIError(w, requestIDFor(r), "campaign_error", err.Error(), status)
}
