package service

import (
	"context"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type executionObservationReader interface {
	Observations(context.Context, string, *domain.ExecutionSnapshot, time.Time) (*domain.ExecutionObservations, error)
}

func (s *ExecutionSnapshotService) Observations(ctx context.Context, ws, id string) (*domain.ExecutionObservations, error) {
	ctx, m, err := s.authorize(ctx, ws)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.repository.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	if err = executionReadPermission(m, snapshot.Kind); err != nil {
		return nil, err
	}
	if !m.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Contact read access is required for execution observations")
	}
	reader, ok := s.repository.(executionObservationReader)
	if !ok {
		return nil, errors.New("execution observation reader unavailable")
	}
	return reader.Observations(ctx, ws, snapshot, time.Now().UTC())
}
