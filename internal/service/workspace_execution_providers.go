package service

import (
	"context"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type automationProviderArchiveRefresher interface {
	UpdateWorkspaceProviders(context.Context, string, func() error) error
}

func (s *WorkspaceService) SetExecutionSnapshotService(snapshots *ExecutionSnapshotService) {
	s.executionSnapshots = snapshots
}
func (s *WorkspaceService) saveWorkspaceWithExecutionProviders(ctx context.Context, workspace *domain.Workspace) error {
	if s.executionSnapshots == nil {
		return s.repo.Update(ctx, workspace)
	}
	return s.executionSnapshots.UpdateWorkspaceProviders(ctx, workspace.ID, func() error { return s.repo.Update(ctx, workspace) })
}

type providerRefreshStorage interface {
	WithProviderRefreshLock(context.Context, string, func() error) error
	SetProviderRefreshPending(context.Context, string, bool) error
	ProviderRefreshPending(context.Context, string) (bool, error)
}

func (s *ExecutionSnapshotService) UpdateWorkspaceProviders(ctx context.Context, ws string, save func() error) error {
	storage, ok := s.repository.(providerRefreshStorage)
	if !ok {
		return errors.New("execution provider preparation unavailable")
	}
	return storage.WithProviderRefreshLock(ctx, ws, func() error {
		if err := storage.SetProviderRefreshPending(ctx, ws, true); err != nil {
			return errors.New("execution provider preparation unavailable")
		}
		if err := save(); err != nil {
			_ = storage.SetProviderRefreshPending(ctx, ws, false)
			return err
		}
		if err := s.RefreshAutomationProviders(ctx, ws); err != nil {
			return errors.New("settings saved; new journeys are paused while execution providers are prepared")
		}
		return storage.SetProviderRefreshPending(ctx, ws, false)
	})
}
func (s *ExecutionSnapshotService) RetryProviderRefresh(ctx context.Context, ws string) error {
	storage, ok := s.repository.(providerRefreshStorage)
	if !ok {
		return nil
	}
	pending, err := storage.ProviderRefreshPending(ctx, ws)
	if err != nil || !pending {
		return err
	}
	return storage.WithProviderRefreshLock(ctx, ws, func() error {
		if err := s.RefreshAutomationProviders(ctx, ws); err != nil {
			return err
		}
		return storage.SetProviderRefreshPending(ctx, ws, false)
	})
}

// A successful integration/default-provider save publishes fresh archives for future enrollments.
// Waiting instances keep their existing opaque runtime reference.
func (s *ExecutionSnapshotService) RefreshAutomationProviders(ctx context.Context, ws string) error {
	pages, ok := s.repository.(interface {
		ProviderAutomationsPage(context.Context, string, string, int) ([]*domain.Automation, error)
	})
	if !ok {
		return errors.New("execution provider archive index unavailable")
	}
	after := ""
	for {
		automations, err := pages.ProviderAutomationsPage(ctx, ws, after, 100)
		if err != nil {
			return err
		}
		for _, a := range automations {
			if err = s.archiveRuntimeNodes(ctx, ws, a); err != nil {
				return err
			}
			after = a.ID
		}
		if len(automations) < 100 {
			return nil
		}
	}
}
