package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/http/middleware"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"net/http"
	"strconv"
)

type BusinessConfigHandler struct {
	service *service.BusinessConfigService
	secret  func() ([]byte, error)
}

func NewBusinessConfigHandler(s *service.BusinessConfigService, secret func() ([]byte, error)) *BusinessConfigHandler {
	return &BusinessConfigHandler{s, secret}
}
func (h *BusinessConfigHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := middleware.NewAuthMiddleware(h.secret).RequireAuth()
	for _, action := range []string{"list", "get", "save", "archive", "preview", "extendEvents"} {
		mux.Handle("/api/businessDefinitions."+action, auth(http.HandlerFunc(h.handle(action))))
	}
}
func writeBusinessError(w http.ResponseWriter, err error) {
	if writePermissionError(w, err) {
		return
	}
	status := http.StatusInternalServerError
	var validation domain.ValidationError
	if errors.As(err, &validation) {
		status = http.StatusBadRequest
	}
	var missingCustomer *domain.ErrCustomerNotFound
	if errors.Is(err, domain.ErrBusinessDefinitionNotFound) || errors.Is(err, sql.ErrNoRows) || errors.As(err, &missingCustomer) {
		status = http.StatusNotFound
	}
	if errors.Is(err, domain.ErrBusinessDefinitionConflict) || errors.Is(err, domain.ErrEventPayloadConflict) {
		status = http.StatusConflict
	}
	if errors.Is(err, domain.ErrTrackedEventErased) {
		status = http.StatusGone
	}
	var identityConflict *domain.ErrCustomerIdentityConflict
	if errors.As(err, &identityConflict) {
		status = http.StatusConflict
	}
	WriteJSONError(w, err.Error(), status)
}
func (h *BusinessConfigHandler) handle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		workspace := q.Get("workspace_id")
		var result any
		var err error
		if action == "get" || action == "list" {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if action == "get" {
				var d *domain.BusinessDefinition
				d, err = h.service.Get(r.Context(), workspace, q.Get("kind"), q.Get("key"))
				result = map[string]any{"definition": d}
			} else {
				limit, _ := strconv.Atoi(q.Get("limit"))
				offset, _ := strconv.Atoi(q.Get("offset"))
				var ds []domain.BusinessDefinition
				var total int
				ds, total, err = h.service.List(r.Context(), workspace, domain.BusinessDefinitionFilter{Search: q.Get("search"), Kind: q.Get("kind"), Status: q.Get("status"), Limit: limit, Offset: offset})
				result = map[string]any{"definitions": ds, "total": total}
			}
		} else {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				WorkspaceID string                    `json:"workspace_id"`
				Definition  domain.BusinessDefinition `json:"definition"`
				Kind        string                    `json:"kind"`
				Key         string                    `json:"key"`
				Revision    int                       `json:"revision"`
				Sample      map[string]any            `json:"sample"`
				EventKeys   []string                  `json:"event_keys"`
				Schema      map[string]any            `json:"schema"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req) != nil {
				WriteJSONError(w, "Invalid request body", 400)
				return
			}
			switch action {
			case "extendEvents":
				var definitions []domain.BusinessDefinition
				definitions, err = h.service.ExtendEvents(r.Context(), req.WorkspaceID, req.EventKeys, req.Schema)
				result = map[string]any{"definitions": definitions}
			case "save":
				var d *domain.BusinessDefinition
				d, err = h.service.Save(r.Context(), req.WorkspaceID, &req.Definition)
				result = map[string]any{"definition": d}
			case "archive":
				_, err = h.service.Archive(r.Context(), req.WorkspaceID, req.Kind, req.Key, req.Revision)
				result = map[string]any{"success": err == nil}
			case "preview":
				result, err = h.service.Preview(r.Context(), req.WorkspaceID, &req.Definition, req.Sample)
			}
		}
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}
}
