package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/ratelimiter"
)

const (
	deliveryReceiptMaxBodyBytes       int64 = 4 << 20
	twilioCallbackMaxBodyBytes        int64 = 64 << 10
	channelWebhookReceiptMaxBodyBytes int64 = 256 << 10
)

type DeliveryReceiptHandler struct {
	service      domain.DeliveryReceiptService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
	apiEndpoint  string
	rateLimiter  *ratelimiter.RateLimiter
}

func NewDeliveryReceiptHandler(
	service domain.DeliveryReceiptService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	apiEndpoint string,
	rateLimiters ...*ratelimiter.RateLimiter,
) *DeliveryReceiptHandler {
	handler := &DeliveryReceiptHandler{
		service: service, getJWTSecret: getJWTSecret, logger: logger,
		apiEndpoint: strings.TrimRight(apiEndpoint, "/"),
	}
	if len(rateLimiters) > 0 {
		handler.rateLimiter = rateLimiters[0]
	}
	return handler
}

func (h *DeliveryReceiptHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/deliveryReceipts.ingest", authMiddleware.RequireAuth()(http.HandlerFunc(h.handleIngest)))
	mux.HandleFunc("/webhooks/delivery/twilio", h.handleTwilio)
	mux.HandleFunc("/webhooks/delivery/channel", h.handleChannelWebhook)
}

func (h *DeliveryReceiptHandler) handleChannelWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	integrationID := r.URL.Query().Get("integration_id")
	if workspaceID == "" || integrationID == "" {
		WriteJSONError(w, "workspace_id and integration_id are required", http.StatusBadRequest)
		return
	}
	if h.rateLimiter != nil {
		if !h.rateLimiter.Allow("delivery_receipt:channel:ip", getClientIP(r)) ||
			!h.rateLimiter.Allow("delivery_receipt:channel:workspace", workspaceID) {
			w.Header().Set("Retry-After", "60")
			WriteJSONError(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
	}
	timestamp, err := strconv.ParseInt(r.Header.Get("X-Yaoguang-Timestamp"), 10, 64)
	if err != nil {
		WriteJSONError(w, "Invalid X-Yaoguang-Timestamp", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, channelWebhookReceiptMaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteJSONError(w, "Channel Webhook receipt body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		WriteJSONError(w, "Invalid Channel Webhook receipt body", http.StatusBadRequest)
		return
	}
	processor, ok := h.service.(domain.ChannelWebhookReceiptProcessor)
	if !ok {
		WriteJSONError(w, "Channel Webhook receipts are not configured", http.StatusNotImplemented)
		return
	}
	result, err := processor.ProcessChannelWebhookCallback(r.Context(), domain.ChannelWebhookReceiptCallback{
		WorkspaceID: workspaceID, IntegrationID: integrationID, Timestamp: timestamp,
		Nonce: r.Header.Get("X-Yaoguang-Nonce"), Signature: r.Header.Get("X-Yaoguang-Signature"), Body: body,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidChannelWebhookSignature):
			WriteJSONError(w, "Invalid Channel Webhook signature", http.StatusUnauthorized)
		case errors.Is(err, service.ErrChannelWebhookReplay):
			WriteJSONError(w, "Channel Webhook receipt was already processed", http.StatusConflict)
		case errors.Is(err, service.ErrChannelWebhookIntegration):
			WriteJSONError(w, "Channel Webhook integration not found", http.StatusNotFound)
		case errors.Is(err, domain.ErrDeliveryReceiptPayloadConflict):
			WriteJSONError(w, err.Error(), http.StatusConflict)
		default:
			var validationError domain.ValidationError
			if errors.As(err, &validationError) {
				WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
				return
			}
			h.logger.WithField("error", err.Error()).Error("Failed to process Channel Webhook receipt")
			WriteJSONError(w, "Failed to process Channel Webhook receipt", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"receipt": result})
}

func (h *DeliveryReceiptHandler) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, deliveryReceiptMaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request domain.IngestDeliveryReceiptsRequest
	if err := decoder.Decode(&request); err != nil {
		handleDeliveryReceiptDecodeError(w, err, deliveryReceiptMaxBodyBytes)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		handleDeliveryReceiptDecodeError(w, err, deliveryReceiptMaxBodyBytes)
		return
	}
	response, err := h.service.Ingest(r.Context(), &request)
	if err != nil {
		var validationError domain.ValidationError
		if errors.As(err, &validationError) {
			WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
			return
		}
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to ingest delivery receipts")
		WriteJSONError(w, "Failed to ingest delivery receipts", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func handleDeliveryReceiptDecodeError(w http.ResponseWriter, err error, maxBytes int64) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteJSONError(w, fmt.Sprintf("Request body exceeds %d bytes", maxBytes), http.StatusRequestEntityTooLarge)
		return
	}
	if err != io.EOF {
		WriteJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	WriteJSONError(w, "Invalid request body: exactly one JSON object is required", http.StatusBadRequest)
}

func (h *DeliveryReceiptHandler) handleTwilio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	integrationID := r.URL.Query().Get("integration_id")
	if workspaceID == "" || integrationID == "" {
		WriteJSONError(w, "workspace_id and integration_id are required", http.StatusBadRequest)
		return
	}
	if h.rateLimiter != nil {
		if !h.rateLimiter.Allow("delivery_receipt:twilio:ip", getClientIP(r)) ||
			!h.rateLimiter.Allow("delivery_receipt:twilio:workspace", workspaceID) {
			w.Header().Set("Retry-After", "60")
			WriteJSONError(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, twilioCallbackMaxBodyBytes)
	if err := r.ParseForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteJSONError(w, "Twilio callback body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		WriteJSONError(w, "Invalid Twilio callback form", http.StatusBadRequest)
		return
	}
	callbackURL := h.apiEndpoint + r.URL.RequestURI()
	result, err := h.service.ProcessTwilioCallback(r.Context(), domain.TwilioDeliveryCallback{
		WorkspaceID: workspaceID, IntegrationID: integrationID,
		MessageID: r.URL.Query().Get("message_id"), EffectKey: r.URL.Query().Get("effect_key"),
		CallbackURL: callbackURL, Signature: r.Header.Get("X-Twilio-Signature"), Form: map[string][]string(r.PostForm),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidTwilioSignature):
			WriteJSONError(w, "Invalid Twilio signature", http.StatusUnauthorized)
		case errors.Is(err, service.ErrTwilioIntegrationNotFound):
			WriteJSONError(w, "Twilio integration not found", http.StatusNotFound)
		case errors.Is(err, domain.ErrDeliveryReceiptPayloadConflict):
			WriteJSONError(w, err.Error(), http.StatusConflict)
		default:
			var validationError domain.ValidationError
			if errors.As(err, &validationError) {
				WriteJSONError(w, validationError.Error(), http.StatusBadRequest)
				return
			}
			h.logger.WithField("error", err.Error()).Error("Failed to process Twilio delivery callback")
			WriteJSONError(w, "Failed to process Twilio delivery callback", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"receipt": result})
}
