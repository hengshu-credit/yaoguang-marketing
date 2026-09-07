package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"sync"
	"time"
)

type FunnelService struct {
	jobsMu      sync.Mutex
	jobs        map[string]context.CancelFunc
	querySlots  chan struct{}
	repository  domain.FunnelRepository
	definitions domain.BusinessDefinitionRepository
	auth        domain.AuthService
}

func NewFunnelService(r domain.FunnelRepository, d domain.BusinessDefinitionRepository, a domain.AuthService) *FunnelService {
	return &FunnelService{repository: r, definitions: d, auth: a, jobs: map[string]context.CancelFunc{}, querySlots: make(chan struct{}, 4)}
}
func authorizeAnalysis(ctx context.Context, auth domain.AuthService, w string, resource domain.PermissionResource, permission domain.PermissionType) (context.Context, error) {
	if w == "" {
		return nil, domain.NewValidationError("workspace_id is required")
	}
	if auth == nil {
		return nil, &domain.ErrUnauthorized{Message: "Authentication required"}
	}
	c, _, membership, err := auth.AuthenticateUserForWorkspace(ctx, w)
	if err != nil {
		return nil, err
	}
	if membership == nil || !membership.HasPermission(resource, permission) {
		return nil, domain.NewPermissionError(resource, permission, "Insufficient permissions")
	}
	return c, nil
}
func (s *FunnelService) Query(ctx context.Context, q domain.FunnelQuery) (domain.FunnelResult, error) {
	ctx, err := authorizeAnalysis(ctx, s.auth, q.WorkspaceID, domain.PermissionResourceWebAnalytics, domain.PermissionTypeRead)
	if err != nil {
		return domain.FunnelResult{}, err
	}
	var d domain.FunnelDefinition
	if q.Definition != nil {
		d = *q.Definition
	} else {
		saved, err := s.definitions.Get(ctx, q.WorkspaceID, "funnel", q.Key)
		if err != nil {
			return domain.FunnelResult{}, err
		}
		raw, err := json.Marshal(saved.Settings)
		if err != nil {
			return domain.FunnelResult{}, err
		}
		if err = json.Unmarshal(raw, &d); err != nil {
			return domain.FunnelResult{}, domain.NewValidationError(err.Error())
		}
	}
	if err = d.Validate(); err != nil {
		return domain.FunnelResult{}, domain.NewValidationError(err.Error())
	}
	if q.Start.IsZero() || !q.End.After(q.Start) || q.End.Sub(q.Start) > 366*24*time.Hour {
		return domain.FunnelResult{}, domain.NewValidationError("query requires a date range of at most one year")
	}
	if q.Watermark.IsZero() {
		q.Watermark = time.Now().UTC()
	}
	if q.Watermark.After(time.Now().UTC().Add(time.Minute)) {
		return domain.FunnelResult{}, domain.NewValidationError("watermark cannot be in the future")
	}
	if d.SubjectType != "customer" {
		object, err := s.definitions.Get(ctx, q.WorkspaceID, "object_type", d.SubjectType)
		if err != nil {
			return domain.FunnelResult{}, err
		}
		if object.Status != "active" {
			return domain.FunnelResult{}, domain.NewValidationError("object type is not active")
		}
	}
	end := q.End.Add(time.Duration(d.WindowSeconds) * time.Second)
	if end.After(q.Watermark) {
		end = q.Watermark
	}
	eventType := d.SubjectType
	if d.HasLinkedSteps() {
		eventType = "customer"
		reader, ok := s.repository.(interface {
			ObjectRelations(context.Context, string, time.Time, time.Time, time.Time) ([]domain.AnalysisObjectRelation, error)
		})
		if !ok {
			return domain.FunnelResult{}, domain.NewValidationError("object relation reader unavailable")
		}
		q.Relations, err = reader.ObjectRelations(ctx, q.WorkspaceID, q.Start, end, q.Watermark)
		if err != nil {
			return domain.FunnelResult{}, err
		}
	}
	events, err := s.repository.Events(ctx, q.WorkspaceID, eventType, q.Start, end, q.Watermark)
	if err != nil {
		return domain.FunnelResult{}, fmt.Errorf("read funnel ledger: %w", err)
	}
	if d.Scope == "recipients" || d.Scope == "attributed" {
		reader, ok := s.repository.(interface {
			AttributionLinks(context.Context, string, time.Time, time.Time, time.Time) ([]domain.AnalysisTouchpoint, error)
		})
		if !ok {
			return domain.FunnelResult{}, domain.NewValidationError("attribution reader unavailable")
		}
		window := d.AttributionWindowSeconds
		if window == 0 {
			window = d.WindowSeconds
		}
		links, err := reader.AttributionLinks(ctx, q.WorkspaceID, q.Start.Add(-time.Duration(window)*time.Second), end, q.Watermark)
		if err != nil {
			return domain.FunnelResult{}, err
		}
		events = domain.FilterAnalysisScope(events, links, d.Scope, d.RunID, window)
	}
	var history []domain.SubjectStateInterval
	if historical, ok := s.repository.(interface {
		StateHistoryAt(context.Context, string, time.Time, time.Time, time.Time) ([]domain.SubjectStateInterval, error)
	}); ok {
		history, err = historical.StateHistoryAt(ctx, q.WorkspaceID, q.Start, end, q.Watermark)
	} else {
		history, err = s.repository.StateHistory(ctx, q.WorkspaceID, q.Start, end)
	}
	if err != nil {
		return domain.FunnelResult{}, err
	}
	result, err := domain.ComputeFunnelContext(ctx, d, q, events, history)
	if !q.IncludeDetails {
		result.Details = nil
	} else {
		if _, err := authorizeAnalysis(ctx, s.auth, q.WorkspaceID, domain.PermissionResourceCustomers, domain.PermissionTypeRead); err != nil {
			return domain.FunnelResult{}, err
		}
	}
	return result, err
}
