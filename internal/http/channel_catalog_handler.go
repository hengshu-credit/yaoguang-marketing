package http

import (
	"errors"
	"net/http"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type ChannelCatalogHandler struct {
	service      domain.ChannelCatalogService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewChannelCatalogHandler(
	service domain.ChannelCatalogService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *ChannelCatalogHandler {
	return &ChannelCatalogHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (h *ChannelCatalogHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/channels.catalog", authMiddleware.RequireAuth()(http.HandlerFunc(h.handleList)))
}

func (h *ChannelCatalogHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	definitions, err := h.service.List(r.Context(), r.URL.Query().Get("workspace_id"))
	if err != nil {
		var validationError domain.ValidationError
		if errors.As(err, &validationError) {
			WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
			return
		}
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to list channel catalogue")
		WriteJSONError(w, "Failed to list channel catalogue", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"channels": definitions})
}
