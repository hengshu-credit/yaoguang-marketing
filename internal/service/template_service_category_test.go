package service_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

type categoryRepoForTemplateTest struct {
	categories map[string]domain.TemplateCategoryDefinition
}

func (s *categoryRepoForTemplateTest) List(context.Context, string, bool) ([]domain.TemplateCategoryDefinition, error) {
	return nil, nil
}
func (s *categoryRepoForTemplateTest) Get(_ context.Context, _, id string) (*domain.TemplateCategoryDefinition, error) {
	category, ok := s.categories[id]
	if !ok {
		return nil, domain.ErrTemplateCategoryNotFound
	}
	return &category, nil
}
func (s *categoryRepoForTemplateTest) Create(context.Context, string, *domain.TemplateCategoryDefinition) error {
	return nil
}
func (s *categoryRepoForTemplateTest) Update(context.Context, string, *domain.TemplateCategoryDefinition) error {
	return nil
}
func (s *categoryRepoForTemplateTest) Delete(context.Context, string, string) error { return nil }

func TestTemplateServiceRejectsInactiveCategoryForNewTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	templateService, _, _, auth, _ := setupTemplateServiceTest(ctrl)
	templateService.SetTemplateCategoryRepository(&categoryRepoForTemplateTest{categories: map[string]domain.TemplateCategoryDefinition{
		"archived": {ID: "archived", Name: "Archived", Purpose: domain.TemplateCategoryPurposeTransactional, IsActive: false},
	}})
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws1").Return(ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
		WorkspaceID: "ws1", Permissions: domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}},
	}, nil)
	template := &domain.Template{ID: "sms-one", Name: "SMS", Channel: domain.ChannelSMS, Category: "archived", SMS: &domain.SMSTemplate{Body: "Hi"}}

	err := templateService.CreateTemplate(ctx, "ws1", template)
	require.ErrorContains(t, err, "inactive")
}

func TestTemplateServiceStampsResolvedCategoryPurposeBeforeCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	templateService, repo, _, auth, _ := setupTemplateServiceTest(ctrl)
	templateService.SetTemplateCategoryRepository(&categoryRepoForTemplateTest{categories: map[string]domain.TemplateCategoryDefinition{
		"vip": {ID: "vip", Name: "VIP", Purpose: domain.TemplateCategoryPurposeMarketing, IsActive: true},
	}})
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws1").Return(ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
		WorkspaceID: "ws1", Permissions: domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}},
	}, nil)
	repo.EXPECT().CreateTemplate(ctx, "ws1", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
		require.Equal(t, domain.TemplateCategoryPurposeMarketing, template.CategoryPurpose)
		require.Equal(t, "marketing", template.Settings["category_purpose"])
		return nil
	})
	template := &domain.Template{ID: "sms-vip", Name: "VIP", Channel: domain.ChannelSMS, Category: "vip", SMS: &domain.SMSTemplate{Body: "Hi"}}

	require.NoError(t, templateService.CreateTemplate(ctx, "ws1", template))
}
