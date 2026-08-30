package http

import (
	"encoding/json"
	"net/http"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type FrequencyPolicyHandler struct {
	service      domain.FrequencyPolicyManager
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewFrequencyPolicyHandler(service domain.FrequencyPolicyManager, getJWTSecret func() ([]byte, error), logger logger.Logger) *FrequencyPolicyHandler {
	return &FrequencyPolicyHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (h *FrequencyPolicyHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	mux.Handle("/api/frequencyPolicies.list", requireAuth(http.HandlerFunc(h.HandleList)))
	mux.Handle("/api/frequencyPolicies.save", requireAuth(http.HandlerFunc(h.HandleSave)))
}

func (h *FrequencyPolicyHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	policies, err := h.service.ListFrequencyPolicies(r.Context(), workspaceID)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to list frequency policies")
		WriteJSONError(w, "Failed to list frequency policies", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"policies": policies})
}

func (h *FrequencyPolicyHandler) HandleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request domain.SaveFrequencyPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	policy, err := h.service.SaveFrequencyPolicy(r.Context(), request)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"policy": policy})
}
