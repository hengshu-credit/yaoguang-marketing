package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/require"
)

type templateCategoryHTTPStub struct {
	list       []domain.TemplateCategoryDefinition
	listErr    error
	created    *domain.TemplateCategoryDefinition
	createErr  error
	updated    *domain.TemplateCategoryDefinition
	updateErr  error
	deleteErr  error
	lastCreate domain.CreateTemplateCategoryRequest
}

func (s *templateCategoryHTTPStub) List(context.Context, domain.ListTemplateCategoriesRequest) ([]domain.TemplateCategoryDefinition, error) {
	return s.list, s.listErr
}
func (s *templateCategoryHTTPStub) Create(_ context.Context, request domain.CreateTemplateCategoryRequest) (*domain.TemplateCategoryDefinition, error) {
	s.lastCreate = request
	return s.created, s.createErr
}
func (s *templateCategoryHTTPStub) Update(context.Context, domain.UpdateTemplateCategoryRequest) (*domain.TemplateCategoryDefinition, error) {
	return s.updated, s.updateErr
}
func (s *templateCategoryHTTPStub) Delete(context.Context, domain.DeleteTemplateCategoryRequest) error {
	return s.deleteErr
}

func TestTemplateCategoryHandlerListsCategories(t *testing.T) {
	stub := &templateCategoryHTTPStub{list: []domain.TemplateCategoryDefinition{{ID: "marketing", Name: "Marketing"}}}
	handler := NewTemplateCategoryHandler(stub, nil, logger.NewLogger())
	recorder := httptest.NewRecorder()
	handler.handleList(recorder, httptest.NewRequest(http.MethodGet, "/api/templateCategories.list?workspace_id=ws1", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Categories []domain.TemplateCategoryDefinition `json:"categories"`
	}
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.Equal(t, "marketing", response.Categories[0].ID)
}

func TestTemplateCategoryHandlerCreatesCategory(t *testing.T) {
	stub := &templateCategoryHTTPStub{created: &domain.TemplateCategoryDefinition{ID: "vip", Name: "VIP"}}
	handler := NewTemplateCategoryHandler(stub, nil, logger.NewLogger())
	body := bytes.NewBufferString(`{"workspace_id":"ws1","id":"vip","name":"VIP","purpose":"marketing","sort_order":15}`)
	recorder := httptest.NewRecorder()
	handler.handleCreate(recorder, httptest.NewRequest(http.MethodPost, "/api/templateCategories.create", body))
	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Equal(t, "vip", stub.lastCreate.ID)
}

func TestTemplateCategoryHandlerMapsProtectedDeleteToConflict(t *testing.T) {
	handler := NewTemplateCategoryHandler(&templateCategoryHTTPStub{deleteErr: domain.ErrTemplateCategoryInUse}, nil, logger.NewLogger())
	recorder := httptest.NewRecorder()
	handler.handleDelete(recorder, httptest.NewRequest(http.MethodPost, "/api/templateCategories.delete", bytes.NewBufferString(`{"workspace_id":"ws1","id":"vip"}`)))
	require.Equal(t, http.StatusConflict, recorder.Code)
}

func TestTemplateCategoryHandlerMapsValidationPermissionAndUnexpectedErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"validation", domain.NewValidationError("bad"), http.StatusBadRequest},
		{"permission", domain.NewPermissionError(domain.PermissionResourceTemplates, domain.PermissionTypeRead, "denied"), http.StatusForbidden},
		{"unexpected", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewTemplateCategoryHandler(&templateCategoryHTTPStub{listErr: tt.err}, nil, logger.NewLogger())
			recorder := httptest.NewRecorder()
			handler.handleList(recorder, httptest.NewRequest(http.MethodGet, "/api/templateCategories.list?workspace_id=ws1", nil))
			require.Equal(t, tt.want, recorder.Code)
		})
	}
}
