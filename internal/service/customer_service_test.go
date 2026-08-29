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

func TestCustomerServiceGetRequiresCustomerReadPermissionAndAllowsOwner(t *testing.T) {
	tests := []struct {
		name       string
		membership *domain.UserWorkspace
		wantError  bool
	}{
		{name: "member with read", membership: customerMembership(true, false)},
		{name: "owner", membership: &domain.UserWorkspace{Role: "owner"}},
		{name: "member without read", membership: customerMembership(false, true), wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repository := mocks.NewMockCustomerRepository(ctrl)
			workspaceRepository := mocks.NewMockWorkspaceRepository(ctrl)
			auth := mocks.NewMockAuthService(ctrl)
			service, err := NewCustomerService(CustomerServiceDependencies{
				Repository: repository, WorkspaceRepository: workspaceRepository, AuthService: auth, MaxSyncBatchSize: 10_000,
			})
			require.NoError(t, err)
			ctx := context.Background()
			auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace1").Return(ctx, &domain.User{}, tt.membership, nil)
			if !tt.wantError {
				repository.EXPECT().Get(ctx, "workspace1", domain.CustomerLocator{ExternalUserID: "external-1"}).
					Return(&domain.Customer{ID: "customer-1"}, nil)
			}

			customer, err := service.GetCustomer(ctx, &domain.GetCustomerRequest{
				WorkspaceID: "workspace1", Locator: domain.CustomerLocator{ExternalUserID: "external-1"},
			})
			if tt.wantError {
				var permission *domain.PermissionError
				assert.ErrorAs(t, err, &permission)
				assert.Nil(t, customer)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "customer-1", customer.ID)
		})
	}
}

func TestCustomerServiceUpsertAuthenticatesLoadsWorkspaceSequenceAndHashesCanonicalInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockCustomerRepository(ctrl)
	workspaceRepository := mocks.NewMockWorkspaceRepository(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	service, err := NewCustomerService(CustomerServiceDependencies{
		Repository: repository, WorkspaceRepository: workspaceRepository, AuthService: auth, MaxSyncBatchSize: 10_000,
	})
	require.NoError(t, err)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace1").Return(ctx, &domain.User{}, customerMembership(false, true), nil)
	workspaceRepository.EXPECT().GetByID(ctx, "workspace1").Return(&domain.Workspace{ID: "workspace1", Sequence: 42}, nil)

	externalID := " external-1 "
	request := &domain.UpsertCustomerRequest{
		WorkspaceID: "workspace1", IdempotencyKey: "idem-1",
		Customer: domain.CustomerUpsertInput{ExternalUserID: &externalID},
	}
	repository.EXPECT().Upsert(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, command domain.CustomerUpsertCommand) (*domain.CustomerMutationResult, error) {
		assert.Equal(t, uint16(42), command.WorkspaceSequence)
		assert.Equal(t, "external-1", *command.Input.ExternalUserID)
		assert.Regexp(t, `^[0-9a-f]{64}$`, command.PayloadHash)
		return &domain.CustomerMutationResult{CustomerID: "customer-1", Action: "created"}, nil
	})

	result, err := service.UpsertCustomer(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, "customer-1", result.CustomerID)
}

func TestCustomerServiceWriteAuthenticationFailuresStopBeforeRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockCustomerRepository(ctrl)
	workspaceRepository := mocks.NewMockWorkspaceRepository(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	service, err := NewCustomerService(CustomerServiceDependencies{
		Repository: repository, WorkspaceRepository: workspaceRepository, AuthService: auth, MaxSyncBatchSize: 10_000,
	})
	require.NoError(t, err)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace1").Return(ctx, nil, nil, errors.New("invalid token"))
	externalID := "external-1"

	_, err = service.UpsertCustomer(ctx, &domain.UpsertCustomerRequest{
		WorkspaceID: "workspace1", IdempotencyKey: "idem-1", Customer: domain.CustomerUpsertInput{ExternalUserID: &externalID},
	})
	assert.ErrorContains(t, err, "authenticate")
}

func TestCustomerServiceBatchReturnsEveryItemInOrderAndKeepsSiblingSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockCustomerRepository(ctrl)
	workspaceRepository := mocks.NewMockWorkspaceRepository(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	service, err := NewCustomerService(CustomerServiceDependencies{
		Repository: repository, WorkspaceRepository: workspaceRepository, AuthService: auth, MaxSyncBatchSize: 10_000,
	})
	require.NoError(t, err)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace1").Return(ctx, &domain.User{}, customerMembership(false, true), nil).Times(1)
	workspaceRepository.EXPECT().GetByID(ctx, "workspace1").Return(&domain.Workspace{ID: "workspace1", Sequence: 42}, nil).Times(1)
	repository.EXPECT().Upsert(ctx, gomock.Any()).Return(&domain.CustomerMutationResult{CustomerID: "customer-1", Action: "created"}, nil)
	repository.EXPECT().Upsert(ctx, gomock.Any()).Return(nil, &domain.ErrCustomerIdentityConflict{IdentityType: domain.CustomerIdentityEmail})

	first, third := "external-1", "external-3"
	response, err := service.UpsertCustomerBatch(ctx, &domain.CustomerBatchUpsertRequest{
		WorkspaceID: "workspace1",
		Items: []domain.CustomerBatchUpsertItem{
			{IdempotencyKey: "idem-1", Customer: domain.CustomerUpsertInput{ExternalUserID: &first}},
			{Customer: domain.CustomerUpsertInput{}},
			{IdempotencyKey: "idem-3", Customer: domain.CustomerUpsertInput{ExternalUserID: &third}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Accepted)
	assert.Equal(t, 2, response.Failed)
	require.Len(t, response.Results, 3)
	assert.Equal(t, []int{0, 1, 2}, []int{response.Results[0].Index, response.Results[1].Index, response.Results[2].Index})
	assert.Equal(t, "accepted", response.Results[0].Status)
	assert.Equal(t, "validation_error", response.Results[1].Error.Code)
	assert.Equal(t, "identity_conflict", response.Results[2].Error.Code)
}

func TestCustomerServiceBatchLimitIsConfigurable(t *testing.T) {
	ctrl := gomock.NewController(t)
	service, err := NewCustomerService(CustomerServiceDependencies{
		Repository: mocks.NewMockCustomerRepository(ctrl), WorkspaceRepository: mocks.NewMockWorkspaceRepository(ctrl),
		AuthService: mocks.NewMockAuthService(ctrl), MaxSyncBatchSize: 2,
	})
	require.NoError(t, err)
	external := "external"

	_, err = service.UpsertCustomerBatch(context.Background(), &domain.CustomerBatchUpsertRequest{
		WorkspaceID: "workspace1",
		Items: []domain.CustomerBatchUpsertItem{
			{IdempotencyKey: "one", Customer: domain.CustomerUpsertInput{ExternalUserID: &external}},
			{IdempotencyKey: "two", Customer: domain.CustomerUpsertInput{ExternalUserID: &external}},
			{IdempotencyKey: "three", Customer: domain.CustomerUpsertInput{ExternalUserID: &external}},
		},
	})
	var validation domain.ValidationError
	assert.ErrorAs(t, err, &validation)
	assert.ErrorContains(t, err, "2")
}

func customerMembership(read, write bool) *domain.UserWorkspace {
	return &domain.UserWorkspace{Role: "member", Permissions: domain.UserPermissions{
		domain.PermissionResourceCustomers: {Read: read, Write: write},
	}}
}
