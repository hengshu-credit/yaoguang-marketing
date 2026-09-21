package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ConsoleThemeStore interface {
	Set(context.Context, string, *domain.ConsoleThemeSettings) error
}
type ConsoleThemeService struct {
	store ConsoleThemeStore
	auth  domain.AuthService
}

func NewConsoleThemeService(store ConsoleThemeStore, auth domain.AuthService) *ConsoleThemeService {
	return &ConsoleThemeService{store: store, auth: auth}
}
func (s *ConsoleThemeService) Save(ctx context.Context, workspaceID string, value *domain.ConsoleThemeSettings) (*domain.ConsoleThemeSettings, error) {
	if workspaceID == "" || value == nil {
		return nil, domain.NewValidationError("workspace_id and console_theme are required")
	}
	ctx, _, member, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if member == nil || member.Role != "owner" {
		return nil, domain.NewPermissionError(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite, "Only workspace owners can save a shared console theme")
	}
	if err := value.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	if err := s.store.Set(ctx, workspaceID, value); err != nil {
		return nil, err
	}
	return value, nil
}
