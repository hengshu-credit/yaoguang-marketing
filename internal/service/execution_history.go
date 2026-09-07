package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (s *ExecutionSnapshotService) HistoryPage(ctx context.Context, ws, source, before string) (*domain.ExecutionHistoryPage, error) {
	ctx, m, err := s.authorize(ctx, ws)
	if err != nil {
		return nil, err
	}
	if m == nil || !m.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Contact read access is required")
	}
	reader, ok := s.repository.(interface {
		HistoryPage(context.Context, string, string, string) (*domain.ExecutionHistoryPage, error)
	})
	if !ok {
		items, err := s.List(ctx, ws, source)
		return &domain.ExecutionHistoryPage{Items: items}, err
	}
	page, err := reader.HistoryPage(ctx, ws, source, before)
	if err != nil {
		return nil, err
	}
	for _, item := range page.Items {
		if err = executionReadPermission(m, item.Kind); err != nil {
			return nil, err
		}
	}
	return page, nil
}
