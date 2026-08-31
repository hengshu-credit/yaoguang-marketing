package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCatalogServiceListsDefinitionsAfterWorkspaceAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	ctx := context.Background()
	authenticatedCtx := context.WithValue(ctx, domain.UserIDKey, "user-1")
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace-1").Return(
		authenticatedCtx,
		&domain.User{ID: "user-1"},
		&domain.UserWorkspace{WorkspaceID: "workspace-1"},
		nil,
	)

	service, err := NewChannelCatalogService(auth)
	require.NoError(t, err)
	definitions, err := service.List(ctx, "workspace-1")
	require.NoError(t, err)
	require.NotEmpty(t, definitions)
	assert.Equal(t, domain.ChannelEmail, definitions[0].ID)
}

func TestChannelCatalogServiceRejectsMissingWorkspaceBeforeAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	service, err := NewChannelCatalogService(auth)
	require.NoError(t, err)

	_, err = service.List(context.Background(), " ")
	require.Error(t, err)
	var validationError domain.ValidationError
	assert.True(t, errors.As(err, &validationError))
	assert.Equal(t, "validation error: workspace_id is required", err.Error())
}

func TestChannelCatalogServiceDoesNotHideAuthenticationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace-1").Return(nil, nil, nil, errors.New("denied"))
	service, err := NewChannelCatalogService(auth)
	require.NoError(t, err)

	_, err = service.List(ctx, "workspace-1")
	require.ErrorContains(t, err, "authenticate channel catalogue")
}

func TestNewChannelCatalogServiceRequiresAuthenticationDependency(t *testing.T) {
	service, err := NewChannelCatalogService(nil)
	assert.Nil(t, service)
	assert.EqualError(t, err, "channel catalogue authentication dependency is required")
}
