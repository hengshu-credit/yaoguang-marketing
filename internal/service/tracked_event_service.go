package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"strings"
	"time"
)

type TrackedEventService struct {
	repo        domain.TrackedEventRepository
	definitions domain.BusinessDefinitionRepository
	auth        domain.AuthService
	workspaces  domain.WorkspaceRepository
	now         func() time.Time
}

func NewTrackedEventService(repo domain.TrackedEventRepository, definitions domain.BusinessDefinitionRepository, auth domain.AuthService, w domain.WorkspaceRepository) *TrackedEventService {
	return &TrackedEventService{repo: repo, definitions: definitions, auth: auth, workspaces: w, now: time.Now}
}
func (s *TrackedEventService) authorize(ctx context.Context, w string, write bool) (context.Context, error) {
	if w == "" {
		return ctx, domain.NewValidationError("workspace_id is required")
	}
	ctx, _, member, err := s.auth.AuthenticateUserForWorkspace(ctx, w)
	if err != nil {
		return ctx, err
	}
	permission := domain.PermissionTypeRead
	if write {
		permission = domain.PermissionTypeWrite
	}
	if member == nil || !member.HasPermission(domain.PermissionResourceContacts, permission) {
		return ctx, domain.NewPermissionError(domain.PermissionResourceContacts, permission, "Event access denied")
	}
	return ctx, nil
}
func (s *TrackedEventService) Track(ctx context.Context, req domain.TrackEventRequest) (_ *domain.TrackedEventResult, trackErr error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		if trackErr != nil {
			if recorder, ok := s.repo.(interface {
				RecordDiagnostic(context.Context, string, string, string, string, string) error
			}); ok {
				report, _ := req.Event["report_id"].(string)
				code, _ := req.Event["event_code"].(string)
				_ = recorder.RecordDiagnostic(ctx, req.WorkspaceID, req.Source, report, code, trackErr.Error())
			}
		}
	}()
	if strings.TrimSpace(req.Source) == "" || len(req.Source) > 128 {
		return nil, domain.NewValidationError("source is required and at most 128 characters")
	}
	if req.Event == nil {
		return nil, domain.NewValidationError("event is required")
	}
	payload, err := json.Marshal(struct {
		Mapping    string         `json:"mapping_key"`
		Touchpoint string         `json:"touchpoint_token,omitempty"`
		Event      map[string]any `json:"event"`
	}{req.MappingKey, req.TouchpointToken, req.Event})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	if finder, ok := s.repo.(interface {
		FindReceiptByPayload(context.Context, string, string, string) (*domain.TrackedEventResult, error)
	}); ok {
		stored, err := finder.FindReceiptByPayload(ctx, req.WorkspaceID, req.Source, hash)
		if err == nil {
			return stored, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	// Direct report IDs are checked before any mutable definition or mapping is read.
	if reportID, ok := req.Event["report_id"].(string); ok && reportID != "" {
		stored, oldHash, err := s.repo.GetReceipt(ctx, req.WorkspaceID, req.Source, reportID)
		if err == nil {
			if oldHash != hash {
				return nil, domain.ErrEventPayloadConflict
			}
			return stored, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	var mapping *domain.BusinessDefinition
	if req.MappingKey != "" {
		mapping, err = s.definitions.Get(ctx, req.WorkspaceID, "event_mapping", req.MappingKey)
		if err != nil {
			return nil, err
		}
		if mapping.Status != "active" {
			return nil, domain.NewValidationError("event mapping must be active")
		}
	}
	mapped, err := domain.MapTrackedEvent(req.Event, mapping)
	if err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	reportID, _ := mapped["report_id"].(string)
	if reportID != "" {
		stored, oldHash, err := s.repo.GetReceipt(ctx, req.WorkspaceID, req.Source, reportID)
		if err == nil {
			if oldHash != hash {
				return nil, domain.ErrEventPayloadConflict
			}
			return stored, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	code, _ := mapped["event_code"].(string)
	definition, err := s.definitions.Get(ctx, req.WorkspaceID, "event", code)
	if err != nil {
		return nil, err
	}
	if definition.Status != "active" {
		return nil, domain.NewValidationError("event definition must be active")
	}
	if definition.Settings["builtin"] == true {
		return nil, domain.NewValidationError("platform event semantics require the internal producer")
	}
	if allowed, ok := definition.Settings["allowed_sources"].([]any); ok {
		found := false
		for _, source := range allowed {
			if source == req.Source {
				found = true
			}
		}
		if !found {
			return nil, domain.NewValidationError("source is not allowed for this event")
		}
	}
	event, err := domain.NormalizeTrackedEvent(req.Event, definition, mapping, s.now())
	if err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	attributes, _, err := s.definitions.List(ctx, req.WorkspaceID, domain.BusinessDefinitionFilter{Kind: "customer_attribute", Status: "active", Limit: 1000})
	if err != nil {
		return nil, err
	}
	byKey := map[string]domain.BusinessDefinition{}
	for _, d := range attributes {
		byKey[d.Key] = d
	}
	workspace, err := s.workspaces.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if req.TouchpointToken != "" {
		touchpoint, ok := domain.ResolveWebTouchpoint(req.TouchpointToken, workspace.Settings.SecretKey, s.now())
		if !ok {
			return nil, domain.NewValidationError("invalid touchpoint token")
		}
		event.TrustedTouchpointID = touchpoint
	}
	return s.repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: req.WorkspaceID, WorkspaceSequence: workspace.Sequence, Source: req.Source, PayloadHash: hash, Event: event, Definition: definition, Mapping: mapping, Attributes: byKey})
}
func (s *TrackedEventService) Get(ctx context.Context, w, source, reportID string) (*domain.TrackedEventResult, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	result, _, err := s.repo.GetReceipt(ctx, w, source, reportID)
	if err != nil {
		return nil, err
	}
	safe, err := s.redactResults(ctx, w, []domain.TrackedEventResult{*result})
	if err != nil {
		return nil, err
	}
	return &safe[0], nil
}
func (s *TrackedEventService) List(ctx context.Context, w, customerID string, limit int) ([]domain.TrackedEventResult, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	if customerID != "" {
		if _, err = uuid.Parse(customerID); err != nil {
			return nil, domain.NewValidationError("customer_id must be a UUID")
		}
	}
	results, err := s.repo.List(ctx, w, customerID, limit)
	if err != nil {
		return nil, err
	}
	return s.redactResults(ctx, w, results)
}

type TrackedEventBatchResult struct {
	Index  int                        `json:"index"`
	Result *domain.TrackedEventResult `json:"result,omitempty"`
	Error  string                     `json:"error,omitempty"`
}

func (s *TrackedEventService) Batch(ctx context.Context, requests []domain.TrackEventRequest) ([]TrackedEventBatchResult, error) {
	if len(requests) == 0 || len(requests) > 100 {
		return nil, domain.NewValidationError("batch must contain 1 to 100 events")
	}
	results := make([]TrackedEventBatchResult, 0, len(requests))
	for i, req := range requests {
		value, err := s.Track(ctx, req)
		item := TrackedEventBatchResult{Index: i, Result: value}
		if err != nil {
			item.Error = fmt.Sprint(err)
		}
		results = append(results, item)
	}
	return results, nil
}
func (s *TrackedEventService) Diagnostics(ctx context.Context, w, source string, limit int) ([]map[string]any, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	reader, ok := s.repo.(interface {
		ListDiagnostics(context.Context, string, string, int) ([]map[string]any, error)
	})
	if !ok {
		return nil, fmt.Errorf("diagnostic repository unavailable")
	}
	return reader.ListDiagnostics(ctx, w, source, limit)
}
