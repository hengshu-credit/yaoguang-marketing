package http

import (
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"net/http"
)

type SubjectStateHandler struct {
	service *service.SubjectStateService
	secret  func() ([]byte, error)
}

func NewSubjectStateHandler(s *service.SubjectStateService, secret func() ([]byte, error)) *SubjectStateHandler {
	return &SubjectStateHandler{service: s, secret: secret}
}
func (h *SubjectStateHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.secret)
	mux.Handle("/api/subjectStates.list", auth.RequireAuth()(http.HandlerFunc(h.list)))
	mux.Handle("/api/subjectStates.set", auth.RequireAuth()(http.HandlerFunc(h.set)))
	mux.Handle("/api/subjectStates.coverage", auth.RequireAuth()(http.HandlerFunc(h.coverage)))
}
func (h *SubjectStateHandler) coverage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	var q domain.SubjectStateCoverageQuery
	if !decodeAnalysisRequest(w, r, &q) {
		return
	}
	result, err := h.service.Coverage(r.Context(), q)
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}
func (h *SubjectStateHandler) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	q := r.URL.Query()
	states, err := h.service.List(r.Context(), q.Get("workspace_id"), q.Get("subject_type"), q.Get("subject_id"), q.Get("history") == "true")
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]interface{}{"states": states})
}
func (h *SubjectStateHandler) set(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, requestIDFor(r), "method_not_allowed", "Method not allowed", 405)
		return
	}
	var q service.SubjectStateSetRequest
	if !decodeAnalysisRequest(w, r, &q) {
		return
	}
	if err := h.service.Set(r.Context(), q); err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]interface{}{"success": true})
}
