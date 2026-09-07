package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type SubjectStateService struct {
	repository  domain.SubjectStateRepository
	events      domain.FunnelRepository
	definitions domain.BusinessDefinitionRepository
	auth        domain.AuthService
}

func NewSubjectStateService(r domain.SubjectStateRepository, e domain.FunnelRepository, d domain.BusinessDefinitionRepository, a domain.AuthService) *SubjectStateService {
	return &SubjectStateService{repository: r, events: e, definitions: d, auth: a}
}

type SubjectStateSetRequest struct {
	WorkspaceID string               `json:"workspace_id"`
	SubjectType string               `json:"subject_type"`
	SubjectID   string               `json:"subject_id"`
	Override    domain.StateOverride `json:"override"`
}

func (s *SubjectStateService) List(ctx context.Context, w, typ, id string, history bool) ([]domain.SubjectStateInterval, error) {
	ctx, err := authorizeAnalysis(ctx, s.auth, w, domain.PermissionResourceCustomers, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	if typ == "" || id == "" {
		return nil, domain.NewValidationError("subject_type and subject_id are required")
	}
	return s.repository.List(ctx, w, typ, id, history)
}
func (s *SubjectStateService) Set(ctx context.Context, q SubjectStateSetRequest) error {
	ctx, err := authorizeAnalysis(ctx, s.auth, q.WorkspaceID, domain.PermissionResourceCustomers, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	d, err := s.definitions.Get(ctx, q.WorkspaceID, "state", q.Override.StateKey)
	if err != nil {
		return err
	}
	if d.Settings["subject_type"] != q.SubjectType {
		return domain.NewValidationError("state subject type does not match")
	}
	if _, err = s.repository.Context(ctx, q.WorkspaceID, q.SubjectType, q.SubjectID); err != nil {
		return err
	}
	actor, _ := ctx.Value(domain.UserIDKey).(string)
	if actor == "" {
		return &domain.ErrUnauthorized{Message: "Authentication required"}
	}
	q.Override.Actor = actor
	q.Override.ValidFrom = time.Now().UTC()
	ctx = domain.WithStateTransitionEmission(ctx, domain.StateTransitionEmission{Enabled: true, AllowInitial: true})
	apply := func(locked context.Context) error {
		if err := s.repository.SetOverride(locked, q.WorkspaceID, q.SubjectType, q.SubjectID, q.Override); err != nil {
			return err
		}
		return s.recomputeLocked(locked, q.WorkspaceID, q.SubjectType, q.SubjectID)
	}
	if locker, ok := s.repository.(interface {
		WithSubjectLock(context.Context, string, string, string, func(context.Context) error) error
	}); ok {
		return locker.WithSubjectLock(ctx, q.WorkspaceID, q.SubjectType, q.SubjectID, apply)
	}
	return apply(ctx)
}
func (s *SubjectStateService) Recompute(ctx context.Context, w, typ, id string) error {
	if locker, ok := s.repository.(interface {
		WithSubjectLock(context.Context, string, string, string, func(context.Context) error) error
	}); ok {
		return locker.WithSubjectLock(ctx, w, typ, id, func(locked context.Context) error { return s.recomputeLocked(locked, w, typ, id) })
	}
	return s.recomputeLocked(ctx, w, typ, id)
}

func (s *SubjectStateService) recomputeLocked(ctx context.Context, w, typ, id string) error {
	defs := []domain.SubjectStateDefinition{}
	offset := 0
	for {
		records, total, err := s.definitions.List(ctx, w, domain.BusinessDefinitionFilter{Kind: "state", Status: "active", Limit: 100, Offset: offset})
		if err != nil {
			return err
		}
		for _, record := range records {
			var d domain.SubjectStateDefinition
			raw, err := json.Marshal(record.Settings)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &d); err != nil {
				return err
			}
			d.Key = record.Key
			d.Revision = record.Revision
			if d.SubjectType == typ {
				defs = append(defs, d)
			}
		}
		offset += len(records)
		if offset >= total || len(records) == 0 {
			break
		}
	}
	if len(defs) == 0 {
		return s.repository.ReplaceProjection(ctx, w, typ, id, nil, nil)
	}
	eval, err := s.repository.Context(ctx, w, typ, id)
	if err != nil {
		return err
	}
	var events []domain.AnalysisEvent
	if reader, ok := s.events.(interface {
		EventsForSubject(context.Context, string, string, string, time.Time, time.Time, time.Time) ([]domain.AnalysisEvent, error)
	}); ok {
		events, err = reader.EventsForSubject(ctx, w, typ, id, time.Unix(0, 0).UTC(), eval.At, eval.At)
	} else {
		events, err = s.events.Events(ctx, w, typ, time.Unix(0, 0).UTC(), eval.At, eval.At)
	}
	if err != nil {
		return err
	}
	customerID := ""
	eval.Events = nil
	for _, e := range events {
		if (typ == "customer" && e.CustomerID == id) || (typ != "customer" && e.SubjectID == id) {
			eval.Events = append(eval.Events, e)
			customerID = e.CustomerID
		}
	}
	if typ == "customer" {
		customerID = id
	}
	overrides, err := s.repository.Overrides(ctx, w, typ, id)
	if err != nil {
		return err
	}
	intervals, next, err := domain.BuildSubjectStateTimeline(defs, eval, overrides, typ, id, customerID)
	if err != nil {
		return err
	}
	previous, err := s.repository.List(ctx, w, typ, id, true)
	if err != nil {
		return err
	}
	if len(eval.AttributeHistory) == 0 {
		intervals = domain.MergeObservedStateHistory(defs, previous, intervals, eval.At)
	}
	return s.repository.ReplaceProjection(ctx, w, typ, id, intervals, next)
}

// ProcessEvent is called after the ledger transaction and before dependent triggers.
// It only projects state; replay and expiry cannot send marketing messages.
func (s *SubjectStateService) ProcessEvent(ctx context.Context, w string, e domain.EventEnvelope) error {
	var payload struct {
		Backfill         bool       `json:"backfill"`
		ProcessingStatus string     `json:"processing_status"`
		ExpiresAt        *time.Time `json:"expires_at"`
		Depth            int        `json:"causation_depth"`
	}
	if err := json.Unmarshal(e.Data, &payload); err != nil {
		return err
	}
	emit := !payload.Backfill && (payload.ProcessingStatus == "" || payload.ProcessingStatus == "accepted") && (payload.ExpiresAt == nil || time.Now().Before(*payload.ExpiresAt))
	ctx = domain.WithStateTransitionEmission(ctx, domain.StateTransitionEmission{Enabled: emit, AllowInitial: emit, Cause: &e, Depth: payload.Depth})
	if err := s.Recompute(ctx, w, e.Subject.Type, e.Subject.ID); err != nil {
		return err
	}
	if e.Subject.CustomerID != "" && e.Subject.Type != "customer" {
		return s.Recompute(ctx, w, "customer", e.Subject.CustomerID)
	}
	return nil
}
func (s *SubjectStateService) Tick(ctx context.Context, w string, at time.Time) error {
	ctx = domain.WithStateTransitionEmission(ctx, domain.StateTransitionEmission{Enabled: true})
	subjects, err := s.repository.Due(ctx, w, at, 100)
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		if err = s.Recompute(ctx, w, subject.Type, subject.ID); err != nil {
			return fmt.Errorf("recompute %s/%s: %w", subject.Type, subject.ID, err)
		}
	}
	return nil
}
