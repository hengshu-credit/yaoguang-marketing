package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customerReconciliationRepositoryStub struct {
	jobType domain.CustomerReconciliationJobType
	getID   string
}

func (stub *customerReconciliationRepositoryStub) Run(_ context.Context, _ string, jobType domain.CustomerReconciliationJobType, batchSize int) (*domain.CustomerReconciliationRun, error) {
	stub.jobType = jobType
	return &domain.CustomerReconciliationRun{ID: "run-1", JobType: jobType, BatchSize: batchSize}, nil
}

func (stub *customerReconciliationRepositoryStub) Get(_ context.Context, _, runID string) (*domain.CustomerReconciliationRun, error) {
	stub.getID = runID
	return &domain.CustomerReconciliationRun{ID: runID}, nil
}

func TestCustomerReconciliationServiceRequiresCustomersWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	repository := &customerReconciliationRepositoryStub{}
	service, err := NewCustomerReconciliationService(repository, auth)
	require.NoError(t, err)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace-1").
		Return(ctx, &domain.User{}, customerMembership(true, false), nil)

	run, err := service.Scan(ctx, &domain.CustomerReconciliationRequest{WorkspaceID: "workspace-1"})
	assert.Nil(t, run)
	var permission *domain.PermissionError
	assert.ErrorAs(t, err, &permission)
}

func TestCustomerReconciliationServiceRunsScanRepairAndGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	repository := &customerReconciliationRepositoryStub{}
	service, err := NewCustomerReconciliationService(repository, auth)
	require.NoError(t, err)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "workspace-1").
		Return(ctx, &domain.User{}, customerMembership(false, true), nil).Times(3)

	_, err = service.Scan(ctx, &domain.CustomerReconciliationRequest{WorkspaceID: " workspace-1 "})
	require.NoError(t, err)
	assert.Equal(t, domain.CustomerReconciliationScan, repository.jobType)
	_, err = service.Repair(ctx, &domain.CustomerReconciliationRequest{WorkspaceID: "workspace-1"})
	require.NoError(t, err)
	assert.Equal(t, domain.CustomerReconciliationRepair, repository.jobType)
	_, err = service.Get(ctx, &domain.CustomerReconciliationGetRequest{WorkspaceID: "workspace-1", RunID: "11111111-1111-4111-8111-111111111111"})
	require.NoError(t, err)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", repository.getID)
}
