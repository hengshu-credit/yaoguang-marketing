package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTemplateCategoryRepositoryListsOrderedCategoriesWithUsage(t *testing.T) {
	db, mockSQL, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	workspaceRepo := new(MockWorkspaceRepository)
	workspaceRepo.On("GetConnection", context.Background(), "ws1").Return(db, nil).Once()
	repo := NewTemplateCategoryRepository(workspaceRepo)
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	mockSQL.ExpectQuery("SELECT c.id, c.name, c.purpose").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "purpose", "sort_order", "is_system", "is_active", "created_at", "updated_at", "usage_count",
	}).AddRow("marketing", "Marketing", "marketing", 10, true, true, now, now, 3).
		AddRow("vip", "VIP", "transactional", 20, false, true, now, now, 1))

	categories, err := repo.List(context.Background(), "ws1", false)
	require.NoError(t, err)
	require.Len(t, categories, 2)
	require.Equal(t, int64(3), categories[0].UsageCount)
	require.Equal(t, "vip", categories[1].ID)
	require.NoError(t, mockSQL.ExpectationsWereMet())
}

func TestTemplateCategoryRepositoryRejectsDeletingUsedCategory(t *testing.T) {
	db, mockSQL, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	workspaceRepo := new(MockWorkspaceRepository)
	workspaceRepo.On("GetConnection", context.Background(), "ws1").Return(db, nil).Once()
	repo := NewTemplateCategoryRepository(workspaceRepo)

	mockSQL.ExpectBegin()
	mockSQL.ExpectQuery("SELECT is_system FROM template_categories").WithArgs("vip").WillReturnRows(sqlmock.NewRows([]string{"is_system"}).AddRow(false))
	mockSQL.ExpectQuery("SELECT COUNT\\(DISTINCT id\\) FROM templates").WithArgs("vip").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mockSQL.ExpectRollback()

	err = repo.Delete(context.Background(), "ws1", "vip")
	require.True(t, errors.Is(err, domain.ErrTemplateCategoryInUse))
	require.NoError(t, mockSQL.ExpectationsWereMet())
}
