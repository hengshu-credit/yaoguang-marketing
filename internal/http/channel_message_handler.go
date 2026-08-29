package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

const channelMessageMaxBodyBytes int64 = 512 << 10

type ChannelMessageHandler struct {
	service      domain.ChannelMessageService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewChannelMessageHandler(service domain.ChannelMessageService, getJWTSecret func() ([]byte, error), logger logger.Logger) *ChannelMessageHandler {
	return &ChannelMessageHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (h *ChannelMessageHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/channelMessages.send", authMiddleware.RequireAuth()(http.HandlerFunc(h.handleSend)))
}

func (h *ChannelMessageHandler) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, channelMessageMaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request domain.SendChannelMessageRequest
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			WriteJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		WriteJSONError(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			WriteJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		WriteJSONError(w, "Invalid request: exactly one JSON object is required", http.StatusBadRequest)
		return
	}
	response, err := h.service.Send(r.Context(), &request)
	if err != nil {
		var validationError domain.ValidationError
		if errors.As(err, &validationError) {
			WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
			return
		}
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to send channel message")
		if response != nil {
			writeJSON(w, http.StatusBadGateway, response)
			return
		}
		WriteJSONError(w, "Failed to send channel message", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
