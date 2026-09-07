package service

import (
	"context"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (s *DeliveryReceiptService) SetExecutionSnapshotService(snapshots *ExecutionSnapshotService) {
	s.executionSnapshots = snapshots
}
func (s *DeliveryReceiptService) callbackProviderWorkspace(ctx context.Context, ws, integrationID, messageID, effectKey, providerMessageID string, current *domain.Workspace) (*domain.Workspace, error) {
	if s.executionSnapshots == nil {
		return current, nil
	}
	storage, ok := s.executionSnapshots.repository.(interface {
		SnapshotForChannelSend(context.Context, string, string, string, string, string) (*domain.ExecutionSnapshot, error)
	})
	if !ok {
		return nil, errors.New("execution provider archive unavailable")
	}
	snapshot, err := storage.SnapshotForChannelSend(ctx, ws, integrationID, messageID, effectKey, providerMessageID)
	if errors.Is(err, domain.ErrExecutionSnapshotNotFound) {
		return current, nil
	}
	if err != nil {
		return nil, err
	}
	bound, err := s.executionSnapshots.bindRuntimeProviders(ctx, ws, snapshot)
	if err != nil {
		return nil, err
	}
	return domain.ExecutionProviderWorkspace(bound, current), nil
}
