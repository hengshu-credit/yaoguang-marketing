package http

import (
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactEndpointHandler struct {
	service      domain.ContactEndpointService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewContactEndpointHandler(
	service domain.ContactEndpointService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *ContactEndpointHandler {
	return &ContactEndpointHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (h *ContactEndpointHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/contactEndpoints.list", authMiddleware.RequireAuth()(http.HandlerFunc(h.handleList)))
}

func (h *ContactEndpointHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request := &domain.ListContactEndpointsRequest{
		WorkspaceID: r.URL.Query().Get("workspace_id"),
		Email:       r.URL.Query().Get("email"),
		Channel:     r.URL.Query().Get("channel"),
	}
	if err := request.Validate(); err != nil {
		WriteJSONError(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	endpoints, err := h.service.List(r.Context(), request)
	if err != nil {
		var validationError domain.ValidationError
		if errors.As(err, &validationError) {
			WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
			return
		}
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to list contact endpoints")
		WriteJSONError(w, "Failed to list contact endpoints", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"endpoints": endpoints})
}
