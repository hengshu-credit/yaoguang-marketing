package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/require"
)

type templateCategoryRepositoryStub struct {
	categories map[string]domain.TemplateCategoryDefinition
}

func (s *templateCategoryRepositoryStub) List(context.Context, string, bool) ([]domain.TemplateCategoryDefinition, error) {
	result := make([]domain.TemplateCategoryDefinition, 0, len(s.categories))
	for _, category := range s.categories {
		result = append(result, category)
	}
	return result, nil
}
func (s *templateCategoryRepositoryStub) Get(_ context.Context, _, id string) (*domain.TemplateCategoryDefinition, error) {
	category, ok := s.categories[id]
	if !ok {
		return nil, domain.ErrTemplateCategoryNotFound
	}
	return &category, nil
}
func (s *templateCategoryRepositoryStub) Create(_ context.Context, _ string, category *domain.TemplateCategoryDefinition) error {
	s.categories[category.ID] = *category
	return nil
}
func (s *templateCategoryRepositoryStub) Update(_ context.Context, _ string, category *domain.TemplateCategoryDefinition) error {
	s.categories[category.ID] = *category
	return nil
}
func (s *templateCategoryRepositoryStub) Delete(_ context.Context, _ string, id string) error {
	delete(s.categories, id)
	return nil
}

func TestTemplateCategoryServiceCreatesCategoryWithTemplateWritePermission(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws1").Return(ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
		WorkspaceID: "ws1", Permissions: domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}},
	}, nil)
	repo := &templateCategoryRepositoryStub{categories: map[string]domain.TemplateCategoryDefinition{}}
	service, err := NewTemplateCategoryService(repo, auth, nil)
	require.NoError(t, err)

	created, err := service.Create(ctx, domain.CreateTemplateCategoryRequest{
		WorkspaceID: "ws1", ID: "vip", Name: "VIP", Purpose: domain.TemplateCategoryPurposeMarketing, SortOrder: 15,
	})
	require.NoError(t, err)
	require.Equal(t, "vip", created.ID)
	require.True(t, created.IsActive)
	require.False(t, created.IsSystem)
}

func TestTemplateCategoryServiceRejectsListWithoutTemplateReadPermission(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws1").Return(ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{WorkspaceID: "ws1"}, nil)
	service, err := NewTemplateCategoryService(&templateCategoryRepositoryStub{categories: map[string]domain.TemplateCategoryDefinition{}}, auth, nil)
	require.NoError(t, err)

	_, err = service.List(ctx, domain.ListTemplateCategoriesRequest{WorkspaceID: "ws1"})
	require.ErrorContains(t, err, "read access")
}
