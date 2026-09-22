package http

import (
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"net/http"
	"strconv"
)

type TrackedEventHandler struct {
	service *service.TrackedEventService
	secret  func() ([]byte, error)
}

func NewTrackedEventHandler(s *service.TrackedEventService, secret func() ([]byte, error)) *TrackedEventHandler {
	return &TrackedEventHandler{s, secret}
}
func (h *TrackedEventHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.secret).RequireAuth()
	for _, prefix := range []string{"events", "trackedEvents"} {
		for _, action := range []string{"track", "batch", "get", "list", "diagnostics", "pending", "resolvePending"} {
			mux.Handle("/api/"+prefix+"."+action, auth(http.HandlerFunc(h.handle(action))))
		}
	}
}
func (h *TrackedEventHandler) handle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var result any
		var err error
		if action == "resolvePending" {
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			var req service.ResolvePendingEventRequest
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
				WriteJSONError(w, "Invalid request body", 400)
				return
			}
			result, err := h.service.ResolvePending(r.Context(), req)
			if err != nil {
				writeBusinessError(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		if action == "diagnostics" {
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			diagnostics, err := h.service.Diagnostics(r.Context(), q.Get("workspace_id"), q.Get("source"), limit)
			if err != nil {
				writeBusinessError(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"diagnostics": diagnostics})
			return
		}
		if action == "pending" {
			if r.Method != http.MethodGet {
				w.WriteHeader(405)
				return
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			pending, pendingErr := h.service.Pending(r.Context(), r.URL.Query().Get("workspace_id"), limit)
			if pendingErr != nil {
				writeBusinessError(w, pendingErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"events": pending})
			return
		}
		if action == "get" || action == "list" {
			if r.Method != http.MethodGet {
				w.WriteHeader(405)
				return
			}
			q := r.URL.Query()
			if action == "get" {
				result, err = h.service.Get(r.Context(), q.Get("workspace_id"), q.Get("source"), q.Get("report_id"))
			} else {
				limit, _ := strconv.Atoi(q.Get("limit"))
				var events []domain.TrackedEventResult
				events, err = h.service.List(r.Context(), q.Get("workspace_id"), q.Get("customer_id"), limit)
				result = map[string]any{"events": events}
			}
		} else {
			if r.Method != http.MethodPost {
				w.WriteHeader(405)
				return
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
			if action == "track" {
				var req domain.TrackEventRequest
				if decoder.Decode(&req) != nil {
					WriteJSONError(w, "Invalid request body", 400)
					return
				}
				result, err = h.service.Track(r.Context(), req)
			} else {
				var req struct {
					Events []domain.TrackEventRequest `json:"events"`
				}
				if decoder.Decode(&req) != nil {
					WriteJSONError(w, "Invalid request body", 400)
					return
				}
				results, e := h.service.Batch(r.Context(), req.Events)
				err = e
				result = map[string]any{"results": results}
			}
		}
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}
}
