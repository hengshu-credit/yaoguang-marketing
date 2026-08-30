package http

import (
	"errors"
	"net/http"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type CustomerReconciliationHandler struct {
	service      domain.CustomerReconciliationService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewCustomerReconciliationHandler(
	service domain.CustomerReconciliationService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *CustomerReconciliationHandler {
	return &CustomerReconciliationHandler{service: service, getJWTSecret: getJWTSecret, logger: logger}
}

func (handler *CustomerReconciliationHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(handler.getJWTSecret)
	auth.ErrorWriter = func(w http.ResponseWriter, r *http.Request, message string, status int) {
		code := "unauthorized"
		if status == http.StatusServiceUnavailable {
			code = "authentication_unavailable"
		}
		writeAPIError(w, requestIDFor(r), code, message, status)
	}
	mux.Handle("/api/customers.reconciliation.scan", auth.RequireAuth()(http.HandlerFunc(handler.handleScan)))
	mux.Handle("/api/customers.reconciliation.repair", auth.RequireAuth()(http.HandlerFunc(handler.handleRepair)))
	mux.Handle("/api/customers.reconciliation.get", auth.RequireAuth()(http.HandlerFunc(handler.handleGet)))
}

func (handler *CustomerReconciliationHandler) handleScan(w http.ResponseWriter, r *http.Request) {
	handler.handleRun(w, r, domain.CustomerReconciliationScan)
}

func (handler *CustomerReconciliationHandler) handleRepair(w http.ResponseWriter, r *http.Request) {
	handler.handleRun(w, r, domain.CustomerReconciliationRepair)
}

func (handler *CustomerReconciliationHandler) handleRun(w http.ResponseWriter, r *http.Request, jobType domain.CustomerReconciliationJobType) {
	requestID := requestIDFor(r)
	if !requireCustomerPost(w, r, requestID) {
		return
	}
	request := &domain.CustomerReconciliationRequest{}
	if !decodeCustomerRequest(w, r, requestID, request) {
		return
	}
	var run *domain.CustomerReconciliationRun
	var err error
	if jobType == domain.CustomerReconciliationRepair {
		run, err = handler.service.Repair(r.Context(), request)
	} else {
		run, err = handler.service.Scan(r.Context(), request)
	}
	if err != nil {
		handler.writeError(w, requestID, err)
		return
	}
	handler.writeRun(w, requestID, run)
}

func (handler *CustomerReconciliationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFor(r)
	if r.Method != http.MethodGet {
		writeAPIError(w, requestID, "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request := &domain.CustomerReconciliationGetRequest{
		WorkspaceID: r.URL.Query().Get("workspace_id"),
		RunID:       r.URL.Query().Get("run_id"),
	}
	run, err := handler.service.Get(r.Context(), request)
	if err != nil {
		handler.writeError(w, requestID, err)
		return
	}
	handler.writeRun(w, requestID, run)
}

func (handler *CustomerReconciliationHandler) writeRun(w http.ResponseWriter, requestID string, run *domain.CustomerReconciliationRun) {
	w.Header().Set("X-Request-ID", requestID)
	writeJSON(w, http.StatusOK, struct {
		RequestID      string                            `json:"request_id"`
		Reconciliation *domain.CustomerReconciliationRun `json:"reconciliation"`
	}{RequestID: requestID, Reconciliation: run})
}

func (handler *CustomerReconciliationHandler) writeError(w http.ResponseWriter, requestID string, err error) {
	var validation domain.ValidationError
	var permission *domain.PermissionError
	var unauthorized *domain.ErrUnauthorized
	var workspaceNotFound *domain.ErrWorkspaceNotFound
	var runNotFound *domain.ErrCustomerReconciliationNotFound
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
	case errors.As(err, &runNotFound):
		writeAPIError(w, requestID, "reconciliation_not_found", runNotFound.Error(), http.StatusNotFound)
	default:
		handler.logger.WithField("error", err.Error()).Error("Customer reconciliation request failed")
		writeAPIError(w, requestID, "internal_error", "Customer reconciliation request failed", http.StatusInternalServerError)
	}
}
