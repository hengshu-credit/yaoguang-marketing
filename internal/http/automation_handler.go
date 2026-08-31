package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

// AutomationHandler handles HTTP requests for automation management
type AutomationHandler struct {
	service      domain.AutomationService
	audienceRuns AutomationAudienceRunner
	preflight    domain.JourneyPreflightEvaluator
	trace        domain.JourneyTraceReader
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

type AutomationAudienceRunner interface {
	Start(context.Context, service.AutomationAudienceRunRequest) (*service.AutomationAudienceRunResult, error)
}

func (h *AutomationHandler) SetJourneyServices(preflight domain.JourneyPreflightEvaluator, trace domain.JourneyTraceReader) {
	h.preflight = preflight
	h.trace = trace
}

func (h *AutomationHandler) SetAudienceRunService(runs AutomationAudienceRunner) {
	h.audienceRuns = runs
}

// NewAutomationHandler creates a new AutomationHandler
func NewAutomationHandler(service domain.AutomationService, getJWTSecret func() ([]byte, error), logger logger.Logger) *AutomationHandler {
	return &AutomationHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the automation routes on the given mux
func (h *AutomationHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Automation CRUD
	mux.Handle("/api/automations.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/automations.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/automations.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/automations.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/automations.delete", requireAuth(http.HandlerFunc(h.handleDelete)))

	// Automation status management
	mux.Handle("/api/automations.activate", requireAuth(http.HandlerFunc(h.handleActivate)))
	mux.Handle("/api/automations.preflight", requireAuth(http.HandlerFunc(h.handlePreflight)))
	mux.Handle("/api/automations.pause", requireAuth(http.HandlerFunc(h.handlePause)))
	mux.Handle("/api/automations.startAudience", requireAuth(http.HandlerFunc(h.handleStartAudience)))
	mux.Handle("/api/automations.realtimeAssess", requireAuth(http.HandlerFunc(h.handleRealtimeAssess)))
	mux.Handle("/api/automations.realtimeActivatePrimary", requireAuth(http.HandlerFunc(h.handleRealtimeActivatePrimary)))
	mux.Handle("/api/automations.realtimeRestoreLegacy", requireAuth(http.HandlerFunc(h.handleRealtimeRestoreLegacy)))

	// Node executions/debugging
	mux.Handle("/api/automations.nodeExecutions", requireAuth(http.HandlerFunc(h.handleGetContactNodeExecutions)))
	mux.Handle("/api/journeys.instances", requireAuth(http.HandlerFunc(h.handleJourneyInstances)))
	mux.Handle("/api/journeys.trace", requireAuth(http.HandlerFunc(h.handleJourneyTrace)))
}

func (h *AutomationHandler) handleStartAudience(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.audienceRuns == nil {
		WriteJSONError(w, "Automation audience runs are not configured", http.StatusServiceUnavailable)
		return
	}
	var request service.AutomationAudienceRunRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	result, err := h.audienceRuns.Start(r.Context(), request)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		var validation domain.ValidationError
		if errors.As(err, &validation) {
			WriteJSONError(w, validation.Error(), http.StatusBadRequest)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to start automation audience run")
		WriteJSONError(w, "Failed to start automation audience run", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"run": result})
}

func (h *AutomationHandler) handlePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.preflight == nil {
		WriteJSONError(w, "Journey preflight is not configured", http.StatusServiceUnavailable)
		return
	}
	var req domain.JourneyPreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.preflight.PreflightAutomation(r.Context(), req)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		if errors.Is(err, domain.ErrJourneyTraceNotFound) {
			WriteJSONError(w, "Automation not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to preflight automation")
		WriteJSONError(w, "Failed to preflight automation", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"preflight": result})
}

func (h *AutomationHandler) handleRealtimeAssess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req domain.RealtimeCutoverWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	assessment, err := h.service.AssessRealtimeCutover(r.Context(), req.WorkspaceID, req.From, req.To)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, "Failed to assess realtime cutover", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"assessment": assessment})
}

func (h *AutomationHandler) handleJourneyInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.trace == nil {
		WriteJSONError(w, "Journey trace is not configured", http.StatusServiceUnavailable)
		return
	}
	query := r.URL.Query()
	limit, err := parseOptionalInt(query.Get("limit"), 50)
	if err != nil {
		WriteJSONError(w, "limit must be an integer", http.StatusBadRequest)
		return
	}
	offset, err := parseOptionalInt(query.Get("offset"), 0)
	if err != nil {
		WriteJSONError(w, "offset must be an integer", http.StatusBadRequest)
		return
	}
	request := domain.JourneyInstanceListRequest{
		WorkspaceID: query.Get("workspace_id"), AutomationID: query.Get("automation_id"), Status: query.Get("status"), Limit: limit, Offset: offset,
		Locator: domain.JourneyCustomerLocator{CustomerID: query.Get("customer_id"), CustomerNo: query.Get("customer_no"), ExternalUserID: query.Get("external_user_id"), Email: query.Get("email")},
	}
	instances, total, err := h.trace.ListInstances(r.Context(), request)
	if err != nil {
		handleJourneyTraceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instances": instances, "total": total, "limit": request.Limit, "offset": request.Offset})
}

func (h *AutomationHandler) handleJourneyTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.trace == nil {
		WriteJSONError(w, "Journey trace is not configured", http.StatusServiceUnavailable)
		return
	}
	query := r.URL.Query()
	trace, err := h.trace.GetTrace(r.Context(), domain.JourneyTraceRequest{WorkspaceID: query.Get("workspace_id"), JourneyInstanceID: query.Get("journey_instance_id")})
	if err != nil {
		handleJourneyTraceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"trace": trace})
}

func handleJourneyTraceError(w http.ResponseWriter, err error) {
	if writePermissionError(w, err) {
		return
	}
	if errors.Is(err, domain.ErrJourneyTraceNotFound) {
		WriteJSONError(w, "Journey trace not found", http.StatusNotFound)
		return
	}
	message := err.Error()
	if strings.Contains(message, "required") || strings.Contains(message, "exactly one") || strings.Contains(message, "invalid") || strings.Contains(message, "limit") {
		WriteJSONError(w, message, http.StatusBadRequest)
		return
	}
	WriteJSONError(w, "Failed to load journey trace", http.StatusInternalServerError)
}

func parseOptionalInt(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func (h *AutomationHandler) handleRealtimeActivatePrimary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req domain.RealtimeCutoverWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	report, err := h.service.ActivateRealtimePrimary(r.Context(), req.WorkspaceID, req.From, req.To)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		if errors.Is(err, domain.ErrRealtimeCutoverBlocked) {
			WriteJSONError(w, err.Error(), http.StatusConflict)
			return
		}
		WriteJSONError(w, "Failed to activate realtime primary mode", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"report": report})
}

func (h *AutomationHandler) handleRealtimeRestoreLegacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req domain.RealtimeCutoverWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	report, err := h.service.RestoreRealtimeLegacy(r.Context(), req.WorkspaceID)
	if err != nil {
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, "Failed to restore legacy triggers", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"report": report})
}

func (h *AutomationHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Create(r.Context(), req.WorkspaceID, req.Automation); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create automation")
		if writePermissionError(w, err) {
			return
		}
		var validationErr domain.ValidationError
		if errors.As(err, &validationErr) {
			WriteJSONError(w, validationErr.Error(), http.StatusBadRequest)
			return
		}
		WriteJSONError(w, "Failed to create automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"automation": req.Automation,
	})
}

func (h *AutomationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetAutomationRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	automation, err := h.service.Get(r.Context(), req.WorkspaceID, req.AutomationID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get automation")
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, "Failed to get automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"automation": automation,
	})
}

func (h *AutomationHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ListAutomationsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	automations, total, err := h.service.List(r.Context(), req.WorkspaceID, req.ToFilter())
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list automations")
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, "Failed to list automations", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"automations": automations,
		"total":       total,
	})
}

func (h *AutomationHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Update(r.Context(), req.WorkspaceID, req.Automation); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update automation")
		if writePermissionError(w, err) {
			return
		}
		var validationErr domain.ValidationError
		if errors.As(err, &validationErr) {
			WriteJSONError(w, validationErr.Error(), http.StatusBadRequest)
			return
		}
		// An update to a live automation regenerates its trigger, so the same
		// caller-supplied trigger configuration can fail here too.
		var conditionErr *domain.TriggerConditionError
		if errors.As(err, &conditionErr) {
			WriteJSONError(w, conditionErr.Error(), http.StatusBadRequest)
			return
		}
		// A transition that lost a race to another admin: nothing is broken and nothing
		// about the request was wrong, so this is a 409 the caller can retry after
		// reloading — not a 500 and not a 400.
		var conflictErr *domain.AutomationConflictError
		if errors.As(err, &conflictErr) {
			WriteJSONError(w, conflictErr.Error(), http.StatusConflict)
			return
		}
		WriteJSONError(w, "Failed to update automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"automation": req.Automation,
	})
}

func (h *AutomationHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Delete(r.Context(), req.WorkspaceID, req.AutomationID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to delete automation")
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, "Failed to delete automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *AutomationHandler) handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ActivateAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.preflight != nil {
		if err := h.preflight.ValidateAutomationPreflight(r.Context(), domain.JourneyPreflightRequest{
			WorkspaceID: req.WorkspaceID, AutomationID: req.AutomationID,
		}, req.PreflightHash, req.ConfirmWarnings); err != nil {
			if writePermissionError(w, err) {
				return
			}
			if errors.Is(err, domain.ErrJourneyPreflightRequired) ||
				errors.Is(err, domain.ErrJourneyPreflightChanged) ||
				errors.Is(err, domain.ErrJourneyPreflightBlocked) ||
				errors.Is(err, domain.ErrJourneyPreflightWarningConfirmation) {
				WriteJSONError(w, err.Error(), http.StatusBadRequest)
				return
			}
			h.logger.WithField("error", err.Error()).Error("Failed to validate automation preflight")
			WriteJSONError(w, "Failed to validate automation preflight", http.StatusInternalServerError)
			return
		}
	}

	if err := h.service.Activate(r.Context(), req.WorkspaceID, req.AutomationID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to activate automation")
		if writePermissionError(w, err) {
			return
		}
		var validationErr domain.ValidationError
		if errors.As(err, &validationErr) {
			WriteJSONError(w, validationErr.Error(), http.StatusBadRequest)
			return
		}
		// The trigger configuration is the caller's input, and PostgreSQL's complaint
		// about it is the only thing that makes it diagnosable. errors.As, not a type
		// assertion: the service wraps this on its way up.
		var conditionErr *domain.TriggerConditionError
		if errors.As(err, &conditionErr) {
			WriteJSONError(w, conditionErr.Error(), http.StatusBadRequest)
			return
		}
		// A transition that lost a race to another admin: nothing is broken and nothing
		// about the request was wrong, so this is a 409 the caller can retry after
		// reloading — not a 500 and not a 400.
		var conflictErr *domain.AutomationConflictError
		if errors.As(err, &conflictErr) {
			WriteJSONError(w, conflictErr.Error(), http.StatusConflict)
			return
		}
		WriteJSONError(w, "Failed to activate automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *AutomationHandler) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.PauseAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Pause(r.Context(), req.WorkspaceID, req.AutomationID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to pause automation")
		if writePermissionError(w, err) {
			return
		}
		// A transition that lost a race to another admin: nothing is broken and nothing
		// about the request was wrong, so this is a 409 the caller can retry after
		// reloading — not a 500 and not a 400.
		var conflictErr *domain.AutomationConflictError
		if errors.As(err, &conflictErr) {
			WriteJSONError(w, conflictErr.Error(), http.StatusConflict)
			return
		}
		WriteJSONError(w, "Failed to pause automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *AutomationHandler) handleGetContactNodeExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetContactNodeExecutionsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	contactAutomation, nodeExecutions, err := h.service.GetContactNodeExecutions(r.Context(), req.WorkspaceID, req.AutomationID, req.Email)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get contact node executions")
		if writePermissionError(w, err) {
			return
		}
		WriteJSONError(w, "Failed to get contact node executions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact_automation": contactAutomation,
		"node_executions":    nodeExecutions,
	})
}
