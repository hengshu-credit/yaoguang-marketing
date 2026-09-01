package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type TemplateCategoryService struct {
	repo   domain.TemplateCategoryRepository
	auth   domain.AuthService
	logger logger.Logger
	now    func() time.Time
}

func NewTemplateCategoryService(repo domain.TemplateCategoryRepository, auth domain.AuthService, log logger.Logger) (*TemplateCategoryService, error) {
	if repo == nil {
		return nil, errors.New("template category repository is required")
	}
	if auth == nil {
		return nil, errors.New("template category authentication dependency is required")
	}
	return &TemplateCategoryService{repo: repo, auth: auth, logger: log, now: time.Now}, nil
}

func (s *TemplateCategoryService) authenticate(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, error) {
	authenticated, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("authenticate template category: %w", err)
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceTemplates, permission) {
		return nil, domain.NewPermissionError(domain.PermissionResourceTemplates, permission,
			fmt.Sprintf("Template %s access is required for category management", permission))
	}
	return authenticated, nil
}

func (s *TemplateCategoryService) List(ctx context.Context, request domain.ListTemplateCategoriesRequest) ([]domain.TemplateCategoryDefinition, error) {
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	ctx, err := s.authenticate(ctx, request.WorkspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, request.WorkspaceID, request.IncludeInactive)
}

func (s *TemplateCategoryService) Create(ctx context.Context, request domain.CreateTemplateCategoryRequest) (*domain.TemplateCategoryDefinition, error) {
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	ctx, err := s.authenticate(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.Get(ctx, request.WorkspaceID, request.ID); err == nil {
		return nil, domain.NewValidationError("template category id already exists")
	} else if !errors.Is(err, domain.ErrTemplateCategoryNotFound) {
		return nil, err
	}
	now := s.now().UTC()
	category := &domain.TemplateCategoryDefinition{ID: request.ID, Name: request.Name, Purpose: request.Purpose,
		SortOrder: request.SortOrder, IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.Create(ctx, request.WorkspaceID, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *TemplateCategoryService) Update(ctx context.Context, request domain.UpdateTemplateCategoryRequest) (*domain.TemplateCategoryDefinition, error) {
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	ctx, err := s.authenticate(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	category, err := s.repo.Get(ctx, request.WorkspaceID, request.ID)
	if err != nil {
		return nil, err
	}
	category.Name = request.Name
	category.SortOrder = request.SortOrder
	category.IsActive = request.IsActive
	category.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, request.WorkspaceID, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *TemplateCategoryService) Delete(ctx context.Context, request domain.DeleteTemplateCategoryRequest) error {
	if err := request.Validate(); err != nil {
		return domain.NewValidationError(err.Error())
	}
	ctx, err := s.authenticate(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, request.WorkspaceID, request.ID)
}

var _ domain.TemplateCategoryService = (*TemplateCategoryService)(nil)
