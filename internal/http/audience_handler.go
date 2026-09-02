package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type AudienceHandler struct {
	service      *service.AudienceService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
}

func NewAudienceHandler(service *service.AudienceService, getJWTSecret func() ([]byte, error), log logger.Logger) *AudienceHandler {
	return &AudienceHandler{service: service, getJWTSecret: getJWTSecret, logger: log}
}

func (h *AudienceHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.getJWTSecret)
	mux.Handle("/api/audiences.create", auth.RequireAuth()(http.HandlerFunc(h.create)))
	mux.Handle("/api/audiences.list", auth.RequireAuth()(http.HandlerFunc(h.list)))
	mux.Handle("/api/audiences.get", auth.RequireAuth()(http.HandlerFunc(h.get)))
	mux.Handle("/api/audiences.update", auth.RequireAuth()(http.HandlerFunc(h.update)))
	mux.Handle("/api/audiences.delete", auth.RequireAuth()(http.HandlerFunc(h.delete)))
	mux.Handle("/api/audiences.preview", auth.RequireAuth()(http.HandlerFunc(h.preview)))
	mux.Handle("/api/audiences.matchCustomer", auth.RequireAuth()(http.HandlerFunc(h.matchCustomer)))
	mux.Handle("/api/audiences.build", auth.RequireAuth()(http.HandlerFunc(h.build)))
	mux.Handle("/api/audiences.buildStatus", auth.RequireAuth()(http.HandlerFunc(h.buildStatus)))
	mux.Handle("/api/audiences.members", auth.RequireAuth()(http.HandlerFunc(h.members)))
}

func (h *AudienceHandler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.List(r.Context(), r.URL.Query().Get("workspace_id"), limit, offset)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": total, "limit": limit, "offset": offset})
}

type audienceMutationRequest struct {
	WorkspaceID string                    `json:"workspace_id"`
	AudienceID  string                    `json:"audience_id,omitempty"`
	Name        string                    `json:"name,omitempty"`
	Description string                    `json:"description,omitempty"`
	Kind        domain.AudienceKind       `json:"kind,omitempty"`
	Definition  domain.AudienceExpression `json:"definition"`
}

func (h *AudienceHandler) decode(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAPIError(w, requestIDFor(r), "invalid_request", err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (h *AudienceHandler) create(w http.ResponseWriter, r *http.Request) {
	request := audienceMutationRequest{}
	if !h.decode(w, r, &request) {
		return
	}
	item, err := h.service.Create(r.Context(), service.CreateAudienceRequest{WorkspaceID: request.WorkspaceID, Name: request.Name, Description: request.Description, Kind: request.Kind, Definition: request.Definition})
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *AudienceHandler) get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	item, err := h.service.Get(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("audience_id"))
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AudienceHandler) update(w http.ResponseWriter, r *http.Request) {
	request := audienceMutationRequest{}
	if !h.decode(w, r, &request) {
		return
	}
	item, err := h.service.UpdateDefinition(r.Context(), request.WorkspaceID, request.AudienceID, request.Definition)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AudienceHandler) delete(w http.ResponseWriter, r *http.Request) {
	request := struct {
		WorkspaceID string `json:"workspace_id"`
		AudienceID  string `json:"audience_id"`
	}{}
	if !h.decode(w, r, &request) {
		return
	}
	if err := h.service.Delete(r.Context(), request.WorkspaceID, request.AudienceID); err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *AudienceHandler) preview(w http.ResponseWriter, r *http.Request) {
	request := audienceMutationRequest{}
	if !h.decode(w, r, &request) {
		return
	}
	items, total, err := h.service.Preview(r.Context(), request.WorkspaceID, request.Definition)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"customers": items, "total": total})
}

func (h *AudienceHandler) matchCustomer(w http.ResponseWriter, r *http.Request) {
	request := struct {
		WorkspaceID string `json:"workspace_id"`
		AudienceID  string `json:"audience_id"`
		CustomerID  string `json:"customer_id"`
	}{}
	if !h.decode(w, r, &request) {
		return
	}
	result, err := h.service.MatchesCurrentCustomer(
		r.Context(), request.WorkspaceID, request.AudienceID, request.CustomerID,
	)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AudienceHandler) build(w http.ResponseWriter, r *http.Request) {
	request := struct {
		WorkspaceID string `json:"workspace_id"`
		AudienceID  string `json:"audience_id"`
		Version     int    `json:"version"`
	}{}
	if !h.decode(w, r, &request) {
		return
	}
	buildID, count, err := h.service.Build(r.Context(), request.WorkspaceID, request.AudienceID, request.Version)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"build_id": buildID, "member_count": count, "version": strconv.Itoa(request.Version)})
}

func (h *AudienceHandler) buildStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	item, err := h.service.BuildStatus(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("build_id"))
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AudienceHandler) members(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	parseTime := func(name string) (*time.Time, error) {
		value := r.URL.Query().Get(name)
		if value == "" {
			return nil, nil
		}
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return nil, fmt.Errorf("%s must be RFC3339", name)
		}
		return &parsed, nil
	}
	joinedAfter, err := parseTime("joined_after")
	if err != nil {
		h.error(w, r, err)
		return
	}
	joinedBefore, err := parseTime("joined_before")
	if err != nil {
		h.error(w, r, err)
		return
	}
	query := domain.AudienceMemberQuery{
		ListID: r.URL.Query().Get("list_id"), AudienceID: r.URL.Query().Get("audience_id"),
		BuildID: r.URL.Query().Get("build_id"), Status: r.URL.Query().Get("status"),
		EventName: r.URL.Query().Get("event_name"), JoinedAfter: joinedAfter, JoinedBefore: joinedBefore,
		AttributeKey: r.URL.Query().Get("attribute_key"), AttributeValue: r.URL.Query().Get("attribute_value"),
		After: r.URL.Query().Get("after"), Limit: limit,
	}
	items, next, err := h.service.Members(r.Context(), r.URL.Query().Get("workspace_id"), query)
	if err != nil {
		h.error(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "next": next})
}

func (h *AudienceHandler) error(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.WithField("error", err.Error()).Error("Audience request failed")
	status := http.StatusBadRequest
	if _, ok := err.(*domain.PermissionError); ok {
		status = http.StatusForbidden
	}
	writeAPIError(w, requestIDFor(r), "audience_error", err.Error(), status)
}
