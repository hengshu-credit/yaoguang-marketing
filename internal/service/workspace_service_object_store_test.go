package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
)

func newObjectStoreWorkspaceService(
	t *testing.T,
	objectStore config.ObjectStoreConfig,
) (*WorkspaceService, *mocks.MockWorkspaceRepository, *mocks.MockAuthService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockWorkspaceRepository(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	service := NewWorkspaceService(
		repo,
		nil,
		nil,
		nil,
		nil,
		auth,
		nil,
		&config.Config{
			RootEmail: "root@example.com",
			Realtime: config.RealtimeConfig{
				ObjectStore: objectStore,
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		"secret-key",
		nil,
		nil,
		nil,
	)
	return service, repo, auth
}

func completeObjectStoreConfig() config.ObjectStoreConfig {
	return config.ObjectStoreConfig{
		Provider:       "minio",
		Endpoint:       "http://minio:9000",
		PublicEndpoint: "http://localhost:19002",
		Bucket:         "workspace-assets",
		Region:         "us-east-1",
		AccessKey:      "minio-user",
		SecretKey:      "minio-secret",
		ForcePathStyle: true,
	}
}

func objectStoreTestString(value string) *string {
	return &value
}

func TestWorkspaceServiceListUsesGlobalObjectStoreOnlyForEmptyFileManager(t *testing.T) {
	explicitRegion := "eu-west-1"
	explicit := domain.FileManagerSettings{
		Provider:       "aws",
		Endpoint:       "https://s3.eu-west-1.amazonaws.com",
		Bucket:         "custom-assets",
		Region:         &explicitRegion,
		AccessKey:      "custom-access",
		SecretKey:      "custom-secret",
		ForcePathStyle: false,
	}

	tests := []struct {
		name        string
		objectStore config.ObjectStoreConfig
		stored      domain.FileManagerSettings
		want        domain.FileManagerSettings
	}{
		{
			name:        "empty workspace inherits the complete global store",
			objectStore: completeObjectStoreConfig(),
			want: domain.FileManagerSettings{
				Provider:       "minio",
				Endpoint:       "http://localhost:19002",
				Bucket:         "workspace-assets",
				Region:         objectStoreTestString("us-east-1"),
				AccessKey:      "minio-user",
				SecretKey:      "minio-secret",
				ForcePathStyle: true,
			},
		},
		{
			name:        "explicit workspace settings win",
			objectStore: completeObjectStoreConfig(),
			stored:      explicit,
			want:        explicit,
		},
		{
			name: "incomplete global settings do not create an invalid fallback",
			objectStore: config.ObjectStoreConfig{
				Provider: "minio",
				Endpoint: "http://minio:9000",
				Bucket:   "workspace-assets",
			},
			want: domain.FileManagerSettings{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, repo, auth := newObjectStoreWorkspaceService(t, test.objectStore)
			ctx := context.Background()
			storedWorkspace := &domain.Workspace{
				ID: "workspace-1",
				Settings: domain.WorkspaceSettings{
					FileManager: test.stored,
				},
			}
			auth.EXPECT().AuthenticateUserFromContext(ctx).Return(&domain.User{Email: "root@example.com"}, nil)
			repo.EXPECT().List(ctx).Return([]*domain.Workspace{storedWorkspace}, nil)

			workspaces, err := service.ListWorkspaces(ctx)
			require.NoError(t, err)
			require.Len(t, workspaces, 1)
			assert.Equal(t, test.want, workspaces[0].Settings.FileManager)
			assert.Equal(t, test.stored, storedWorkspace.Settings.FileManager,
				"runtime defaults must not mutate repository-owned workspace state")
		})
	}
}

func TestWorkspaceServiceGetUsesGlobalObjectStoreForExistingEmptyWorkspace(t *testing.T) {
	service, repo, _ := newObjectStoreWorkspaceService(t, completeObjectStoreConfig())
	ctx := context.WithValue(context.Background(), domain.SystemCallKey, true)
	repo.EXPECT().GetByID(ctx, "workspace-1").Return(&domain.Workspace{ID: "workspace-1"}, nil)

	workspace, err := service.GetWorkspace(ctx, "workspace-1")
	require.NoError(t, err)
	assert.Equal(t, "minio", workspace.Settings.FileManager.Provider)
	assert.Equal(t, "http://localhost:19002", workspace.Settings.FileManager.Endpoint)
	assert.Equal(t, "workspace-assets", workspace.Settings.FileManager.Bucket)
	assert.Equal(t, "minio-user", workspace.Settings.FileManager.AccessKey)
	assert.Equal(t, "minio-secret", workspace.Settings.FileManager.SecretKey)
	assert.True(t, workspace.Settings.FileManager.ForcePathStyle)
	require.NotNil(t, workspace.Settings.FileManager.Region)
	assert.Equal(t, "us-east-1", *workspace.Settings.FileManager.Region)
}

func TestWorkspaceServiceCreateReturnsGlobalObjectStoreWithoutPersistingEnvSecret(t *testing.T) {
	service, repo, auth, userService, contactService, listService, taskRepo := newWorkspaceServiceForWebAnalyticsTest(t)
	service.config.Realtime.ObjectStore = completeObjectStoreConfig()

	ctx := context.Background()
	workspaceID := "workspace1"
	owner := &domain.User{ID: "owner", Email: "test@example.com", Name: "Owner"}
	auth.EXPECT().AuthenticateUserFromContext(ctx).Return(owner, nil)
	repo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)

	var persisted domain.FileManagerSettings
	repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, workspace *domain.Workspace) error {
		persisted = workspace.Settings.FileManager
		return nil
	})
	repo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
	userService.EXPECT().GetUserByID(ctx, owner.ID).Return(owner, nil)
	contactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})
	listService.EXPECT().CreateList(ctx, workspaceID, gomock.Any()).Return(nil)
	listService.EXPECT().SubscribeToLists(ctx, gomock.Any(), true).Return(nil)
	taskRepo.EXPECT().List(ctx, workspaceID, gomock.Any()).Return([]*domain.Task{}, 0, nil).AnyTimes()
	taskRepo.EXPECT().Create(ctx, workspaceID, gomock.Any()).Return(nil).AnyTimes()

	workspace, err := service.CreateWorkspace(
		ctx, workspaceID, "Workspace", "https://example.com", "", "", "UTC",
		domain.FileManagerSettings{}, "en", []string{"en"},
	)
	require.NoError(t, err)
	assert.Empty(t, persisted, "environment credentials must not be duplicated into workspace storage")
	assert.Equal(t, "minio", workspace.Settings.FileManager.Provider)
	assert.Equal(t, "http://localhost:19002", workspace.Settings.FileManager.Endpoint)
	assert.Equal(t, "workspace-assets", workspace.Settings.FileManager.Bucket)
	assert.Equal(t, "minio-secret", workspace.Settings.FileManager.SecretKey)
}
