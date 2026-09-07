package http

import (
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"net/http"
)

type BrowserEventHandler struct {
	service *service.BrowserEventService
	secret  func() ([]byte, error)
}

func NewBrowserEventHandler(s *service.BrowserEventService, secret func() ([]byte, error)) *BrowserEventHandler {
	return &BrowserEventHandler{s, secret}
}
func (h *BrowserEventHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.secret).RequireAuth()
	mux.Handle("/api/browserEvents.issueCredential", auth(http.HandlerFunc(h.issue)))
	mux.HandleFunc("/track/events", h.track)
	mux.HandleFunc("/track/identify", h.identify)
}
func (h *BrowserEventHandler) issue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var req service.IssueBrowserCredentialRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req) != nil {
		WriteJSONError(w, "Invalid request", 400)
		return
	}
	token, err := h.service.IssueCredential(r.Context(), req)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"credential": token, "expires_in": 900})
}
func browserEventCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return false
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return false
	}
	return true
}
func (h *BrowserEventHandler) track(w http.ResponseWriter, r *http.Request) {
	if !browserEventCORS(w, r) {
		return
	}
	var req domain.BrowserTrackEventRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&req) != nil {
		WriteJSONError(w, "Invalid request", 400)
		return
	}
	result, err := h.service.Track(r.Context(), req, r.Header.Get("Origin"))
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"report_id": result.ReportID, "event_id": result.EventID, "reported_at": result.ReportedAt, "duplicate": result.Duplicate, "processing_status": result.ProcessingStatus})
}
func (h *BrowserEventHandler) identify(w http.ResponseWriter, r *http.Request) {
	if !browserEventCORS(w, r) {
		return
	}
	var req domain.BrowserTrackEventRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
		WriteJSONError(w, "Invalid request", 400)
		return
	}
	count, err := h.service.Identify(r.Context(), req.WorkspaceID, req.SessionID, req.Credential, r.Header.Get("Origin"))
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"associated": count})
}
