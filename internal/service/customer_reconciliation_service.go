package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const DefaultCustomerReconciliationBatchSize = 2_000

type CustomerReconciliationService struct {
	repository  domain.CustomerReconciliationRepository
	authService domain.AuthService
}

var _ domain.CustomerReconciliationService = (*CustomerReconciliationService)(nil)

func NewCustomerReconciliationService(
	repository domain.CustomerReconciliationRepository,
	authService domain.AuthService,
) (*CustomerReconciliationService, error) {
	if repository == nil || authService == nil {
		return nil, errors.New("customer reconciliation dependencies are required")
	}
	return &CustomerReconciliationService{repository: repository, authService: authService}, nil
}

func (service *CustomerReconciliationService) Scan(ctx context.Context, request *domain.CustomerReconciliationRequest) (*domain.CustomerReconciliationRun, error) {
	return service.run(ctx, request, domain.CustomerReconciliationScan)
}

func (service *CustomerReconciliationService) Repair(ctx context.Context, request *domain.CustomerReconciliationRequest) (*domain.CustomerReconciliationRun, error) {
	return service.run(ctx, request, domain.CustomerReconciliationRepair)
}

func (service *CustomerReconciliationService) run(
	ctx context.Context,
	request *domain.CustomerReconciliationRequest,
	jobType domain.CustomerReconciliationJobType,
) (*domain.CustomerReconciliationRun, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	request.JobType = jobType
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authorizedCtx, err := service.authorize(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return service.repository.Run(authorizedCtx, request.WorkspaceID, jobType, DefaultCustomerReconciliationBatchSize)
}

func (service *CustomerReconciliationService) Get(ctx context.Context, request *domain.CustomerReconciliationGetRequest) (*domain.CustomerReconciliationRun, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authorizedCtx, err := service.authorize(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return service.repository.Get(authorizedCtx, request.WorkspaceID, request.RunID)
}

func (service *CustomerReconciliationService) authorize(ctx context.Context, workspaceID string) (context.Context, error) {
	authorizedCtx, _, membership, err := service.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate customer reconciliation request: %w", err)
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceCustomers, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceCustomers,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to customers required for reconciliation",
		)
	}
	return authorizedCtx, nil
}
