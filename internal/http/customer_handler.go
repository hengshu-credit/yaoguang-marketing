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

const customerMaxBodyBytes int64 = 32 << 20

type CustomerHandler struct {
	service      domain.CustomerService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewCustomerHandler(service domain.CustomerService, getJWTSecret func() ([]byte, error), logger logger.Logger) *CustomerHandler {
	return &CustomerHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (handler *CustomerHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(handler.getJWTSecret)
	auth.ErrorWriter = func(w http.ResponseWriter, r *http.Request, message string, status int) {
		code := "unauthorized"
		if status == http.StatusServiceUnavailable {
			code = "authentication_unavailable"
		}
		writeAPIError(w, requestIDFor(r), code, message, status)
	}
	mux.Handle("/api/customers.get", auth.RequireAuth()(http.HandlerFunc(handler.handleGet)))
	mux.Handle("/api/customers.upsert", auth.RequireAuth()(http.HandlerFunc(handler.handleUpsert)))
	mux.Handle("/api/customers.batch", auth.RequireAuth()(http.HandlerFunc(handler.handleBatch)))
	mux.Handle("/api/customers.merge", auth.RequireAuth()(http.HandlerFunc(handler.handleMerge)))
}

func (handler *CustomerHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFor(r)
	if !requireCustomerPost(w, r, requestID) {
		return
	}
	var request domain.GetCustomerRequest
	if !decodeCustomerRequest(w, r, requestID, &request) {
		return
	}
	customer, err := handler.service.GetCustomer(r.Context(), &request)
	if err != nil {
		handler.writeError(w, requestID, err, "get")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"request_id": requestID, "customer": customer})
}

func (handler *CustomerHandler) handleUpsert(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFor(r)
	if !requireCustomerPost(w, r, requestID) {
		return
	}
	var request domain.UpsertCustomerRequest
	if !decodeCustomerRequest(w, r, requestID, &request) {
		return
	}
	result, err := handler.service.UpsertCustomer(r.Context(), &request)
	if err != nil {
		handler.writeError(w, requestID, err, "upsert")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"request_id": requestID, "customer": result})
}

func (handler *CustomerHandler) handleBatch(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFor(r)
	if !requireCustomerPost(w, r, requestID) {
		return
	}
	var request domain.CustomerBatchUpsertRequest
	if !decodeCustomerRequest(w, r, requestID, &request) {
		return
	}
	result, err := handler.service.UpsertCustomerBatch(r.Context(), &request)
	if err != nil {
		handler.writeError(w, requestID, err, "batch")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	writeJSON(w, http.StatusOK, struct {
		RequestID string                           `json:"request_id"`
		Accepted  int                              `json:"accepted"`
		Failed    int                              `json:"failed"`
		Results   []domain.CustomerBatchItemResult `json:"results"`
	}{RequestID: requestID, Accepted: result.Accepted, Failed: result.Failed, Results: result.Results})
}

func (handler *CustomerHandler) handleMerge(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFor(r)
	if !requireCustomerPost(w, r, requestID) {
		return
	}
	var request domain.CustomerMergeRequest
	if !decodeCustomerRequest(w, r, requestID, &request) {
		return
	}
	result, err := handler.service.MergeCustomer(r.Context(), &request)
	if err != nil {
		handler.writeError(w, requestID, err, "merge")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"request_id": requestID, "merge": result})
}

func requireCustomerPost(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if r.Method == http.MethodPost {
		return true
	}
	writeAPIError(w, requestID, "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

func decodeCustomerRequest(w http.ResponseWriter, r *http.Request, requestID string, target interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, customerMaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAPIError(w, requestID, "request_too_large", "Request body exceeds 32 MiB", http.StatusRequestEntityTooLarge)
			return false
		}
		writeAPIError(w, requestID, "invalid_json", "Invalid JSON request body", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAPIError(w, requestID, "request_too_large", "Request body exceeds 32 MiB", http.StatusRequestEntityTooLarge)
			return false
		}
		writeAPIError(w, requestID, "invalid_json", "Exactly one JSON object is required", http.StatusBadRequest)
		return false
	}
	return true
}

func (handler *CustomerHandler) writeError(w http.ResponseWriter, requestID string, err error, operation string) {
	var validation domain.ValidationError
	var permission *domain.PermissionError
	var unauthorized *domain.ErrUnauthorized
	var workspaceNotFound *domain.ErrWorkspaceNotFound
	var customerNotFound *domain.ErrCustomerNotFound
	var identityConflict *domain.ErrCustomerIdentityConflict
	var externalConflict *domain.ErrCustomerExternalIDConflict
	var numberConflict *domain.ErrCustomerNumberConflict
	var idempotencyConflict *domain.ErrCustomerIdempotencyConflict
	var mergeRejected *domain.ErrCustomerMergeRejected
	switch {
	case errors.As(err, &validation):
		writeAPIError(w, requestID, "validation_error", validation.Error(), http.StatusBadRequest)
	case errors.Is(err, domain.ErrAPIKeyRevoked):
		writeAPIError(w, requestID, "unauthorized", "API key has been revoked", http.StatusUnauthorized)
	case errors.As(err, &permission):
		writeAPIError(w, requestID, "forbidden", permission.Error(), http.StatusForbidden)
	case errors.As(err, &unauthorized), errors.Is(err, domain.ErrUserNotInWorkspace):
		writeAPIError(w, requestID, "forbidden", "You do not have access to this workspace", http.StatusForbidden)
	case errors.As(err, &workspaceNotFound):
		writeAPIError(w, requestID, "workspace_not_found", "Workspace not found", http.StatusNotFound)
	case errors.As(err, &customerNotFound):
		writeAPIError(w, requestID, "customer_not_found", customerNotFound.Error(), http.StatusNotFound)
	case errors.As(err, &identityConflict):
		writeAPIError(w, requestID, "identity_conflict", identityConflict.Error(), http.StatusConflict)
	case errors.As(err, &externalConflict):
		writeAPIError(w, requestID, "external_id_conflict", externalConflict.Error(), http.StatusConflict)
	case errors.As(err, &numberConflict):
		writeAPIError(w, requestID, "customer_number_conflict", numberConflict.Error(), http.StatusConflict)
	case errors.As(err, &idempotencyConflict):
		writeAPIError(w, requestID, "idempotency_conflict", idempotencyConflict.Error(), http.StatusConflict)
	case errors.As(err, &mergeRejected):
		writeAPIError(w, requestID, "merge_rejected", mergeRejected.Error(), http.StatusConflict)
	default:
		handler.logger.WithField("error", err.Error()).WithField("operation", operation).Error("Customer request failed")
		writeAPIError(w, requestID, "internal_error", "Customer request failed", http.StatusInternalServerError)
	}
}
