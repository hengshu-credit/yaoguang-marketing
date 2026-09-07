package http

import (
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"net/http"
	"strconv"
)

type BusinessObjectHandler struct {
	service *service.BusinessObjectService
	secret  func() ([]byte, error)
}

func NewBusinessObjectHandler(s *service.BusinessObjectService, secret func() ([]byte, error)) *BusinessObjectHandler {
	return &BusinessObjectHandler{s, secret}
}
func (h *BusinessObjectHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.secret).RequireAuth()
	for _, action := range []string{"upsert", "get", "list", "relate"} {
		mux.Handle("/api/businessObjects."+action, auth(http.HandlerFunc(h.handle(action))))
	}
}
func (h *BusinessObjectHandler) handle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		var result any
		var err error
		if action == "list" || action == "get" {
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			if action == "get" {
				o, e := h.service.Get(r.Context(), q.Get("workspace_id"), q.Get("object_type"), q.Get("source"), q.Get("external_key"))
				err = e
				result = map[string]any{"object": o}
			} else {
				limit, _ := strconv.Atoi(q.Get("limit"))
				objects, e := h.service.List(r.Context(), q.Get("workspace_id"), q.Get("customer_id"), q.Get("object_type"), limit)
				err = e
				result = map[string]any{"objects": objects}
			}
		} else {
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			var req struct {
				WorkspaceID string                `json:"workspace_id"`
				Object      domain.BusinessObject `json:"object"`
				RelationKey string                `json:"relation_key"`
				FromID      string                `json:"from_id"`
				ToID        string                `json:"to_id"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req) != nil {
				WriteJSONError(w, "Invalid request body", 400)
				return
			}
			if action == "upsert" {
				o, e := h.service.Upsert(r.Context(), req.WorkspaceID, &req.Object)
				err = e
				result = map[string]any{"object": o}
			} else {
				err = h.service.Relate(r.Context(), req.WorkspaceID, req.RelationKey, req.FromID, req.ToID)
				result = map[string]any{"success": err == nil}
			}
		}
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}
}
