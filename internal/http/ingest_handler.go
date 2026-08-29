package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

const ingestMaxBodyBytes int64 = 4 << 20

type IngestHandler struct {
	service      domain.IngestService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewIngestHandler(
	service domain.IngestService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *IngestHandler {
	return &IngestHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (h *IngestHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/ingest.batch", authMiddleware.RequireAuth()(http.HandlerFunc(h.handleBatch)))
}

func (h *IngestHandler) handleBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ingestMaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request domain.IngestBatchRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteJSONError(w, "Request body exceeds 4 MiB", http.StatusRequestEntityTooLarge)
			return
		}
		WriteJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteJSONError(w, "Request body exceeds 4 MiB", http.StatusRequestEntityTooLarge)
			return
		}
		WriteJSONError(w, "Invalid request body: exactly one JSON object is required", http.StatusBadRequest)
		return
	}

	response, err := h.service.IngestBatch(r.Context(), &request)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrIngestBusy):
			w.Header().Set("Retry-After", "1")
			WriteJSONError(w, "Ingest capacity is full; retry with backoff", http.StatusTooManyRequests)
			return
		default:
			var validationError domain.ValidationError
			if errors.As(err, &validationError) {
				WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
				return
			}
			if writeServiceError(w, err, "You do not have access to this workspace") {
				return
			}
			h.logger.WithField("error", err.Error()).Error("Failed to ingest audience batch")
			WriteJSONError(w, "Failed to ingest batch", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}
