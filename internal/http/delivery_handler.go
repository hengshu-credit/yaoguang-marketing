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

type DeliveryHandler struct {
	service      domain.DeliveryManagementService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewDeliveryHandler(service domain.DeliveryManagementService, getJWTSecret func() ([]byte, error), log logger.Logger) *DeliveryHandler {
	return &DeliveryHandler{service: service, getJWTSecret: getJWTSecret, logger: log}
}

func (h *DeliveryHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/deliveries.list", auth.RequireAuth()(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/deliveries.get", auth.RequireAuth()(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/deliveries.reconcile", auth.RequireAuth()(http.HandlerFunc(h.handleReconcile)))
	mux.Handle("/api/deliveries.resolveUnknown", auth.RequireAuth()(http.HandlerFunc(h.handleResolveUnknown)))
}

func (h *DeliveryHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.List(r.Context(), &domain.DeliveryListRequest{
		WorkspaceID: r.URL.Query().Get("workspace_id"), Status: domain.DeliveryStatus(r.URL.Query().Get("status")), Limit: limit, Offset: offset,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deliveries": items, "total": total})
}

func (h *DeliveryHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	detail, err := h.service.Get(r.Context(), &domain.DeliveryGetRequest{WorkspaceID: r.URL.Query().Get("workspace_id"), IntentID: r.URL.Query().Get("intent_id")})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"delivery": detail})
}

func (h *DeliveryHandler) handleReconcile(w http.ResponseWriter, r *http.Request) {
	request := &domain.DeliveryReconcileRequest{}
	if !h.decodePost(w, r, request) {
		return
	}
	if err := h.service.Reconcile(r.Context(), request); err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
}

func (h *DeliveryHandler) handleResolveUnknown(w http.ResponseWriter, r *http.Request) {
	request := &domain.DeliveryResolveUnknownRequest{}
	if !h.decodePost(w, r, request) {
		return
	}
	if err := h.service.ResolveUnknown(r.Context(), request); err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (h *DeliveryHandler) decodePost(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAPIError(w, requestIDFor(r), "invalid_json", "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *DeliveryHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := requestIDFor(r)
	var validation domain.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(w, requestID, "validation_error", validation.Error(), http.StatusBadRequest)
	case errors.Is(err, domain.ErrDeliveryNotFound):
		writeAPIError(w, requestID, "delivery_not_found", "Delivery intent not found", http.StatusNotFound)
	case writePermissionError(w, err):
	default:
		h.logger.WithField("error", err.Error()).Error("Delivery management request failed")
		writeAPIError(w, requestID, "internal_error", "Delivery management request failed", http.StatusInternalServerError)
	}
}
