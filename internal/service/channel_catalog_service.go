package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ChannelCatalogService struct {
	auth domain.AuthService
}

func NewChannelCatalogService(auth domain.AuthService) (*ChannelCatalogService, error) {
	if auth == nil {
		return nil, errors.New("channel catalogue authentication dependency is required")
	}
	return &ChannelCatalogService{auth: auth}, nil
}

func (s *ChannelCatalogService) List(ctx context.Context, workspaceID string) ([]domain.ChannelDefinition, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, domain.NewValidationError("workspace_id is required")
	}
	if _, _, _, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID); err != nil {
		return nil, fmt.Errorf("authenticate channel catalogue: %w", err)
	}
	return domain.ListChannelDefinitions(), nil
}
