package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const DefaultCustomerSyncMaxBatchSize = 10_000

type CustomerServiceDependencies struct {
	Repository          domain.CustomerRepository
	WorkspaceRepository domain.WorkspaceRepository
	AuthService         domain.AuthService
	MaxSyncBatchSize    int
}

type CustomerService struct {
	repository          domain.CustomerRepository
	workspaceRepository domain.WorkspaceRepository
	authService         domain.AuthService
	maxSyncBatchSize    int
}

var _ domain.CustomerService = (*CustomerService)(nil)

func NewCustomerService(dependencies CustomerServiceDependencies) (*CustomerService, error) {
	if dependencies.Repository == nil || dependencies.WorkspaceRepository == nil || dependencies.AuthService == nil {
		return nil, errors.New("customer service dependencies are required")
	}
	if dependencies.MaxSyncBatchSize <= 0 {
		return nil, errors.New("customer synchronous batch limit must be positive")
	}
	return &CustomerService{
		repository: dependencies.Repository, workspaceRepository: dependencies.WorkspaceRepository,
		authService: dependencies.AuthService, maxSyncBatchSize: dependencies.MaxSyncBatchSize,
	}, nil
}

func (s *CustomerService) GetCustomer(ctx context.Context, request *domain.GetCustomerRequest) (*domain.Customer, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authenticatedCtx, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.Get(authenticatedCtx, request.WorkspaceID, request.Locator)
}

func (s *CustomerService) UpsertCustomer(ctx context.Context, request *domain.UpsertCustomerRequest) (*domain.CustomerMutationResult, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authenticatedCtx, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaceRepository.GetByID(authenticatedCtx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load workspace customer sequence: %w", err)
	}
	return s.upsertValidated(authenticatedCtx, workspace, request.IdempotencyKey, request.Customer)
}

func (s *CustomerService) UpsertCustomerBatch(ctx context.Context, request *domain.CustomerBatchUpsertRequest) (*domain.CustomerBatchUpsertResponse, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	if err := request.Validate(s.maxSyncBatchSize); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authenticatedCtx, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaceRepository.GetByID(authenticatedCtx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load workspace customer sequence: %w", err)
	}

	response := &domain.CustomerBatchUpsertResponse{Results: make([]domain.CustomerBatchItemResult, len(request.Items))}
	for index := range request.Items {
		item := &request.Items[index]
		response.Results[index] = domain.CustomerBatchItemResult{Index: index, IdempotencyKey: item.IdempotencyKey, Status: "error"}
		validated := &domain.UpsertCustomerRequest{
			WorkspaceID: request.WorkspaceID, IdempotencyKey: item.IdempotencyKey, Customer: item.Customer,
		}
		if err := validated.Validate(); err != nil {
			response.Results[index].Error = &domain.CustomerBatchItemError{Code: "validation_error", Message: err.Error()}
			response.Failed++
			continue
		}
		result, err := s.upsertValidated(authenticatedCtx, workspace, validated.IdempotencyKey, validated.Customer)
		if err != nil {
			response.Results[index].Error = customerBatchError(err)
			response.Failed++
			continue
		}
		response.Results[index].Status = "accepted"
		response.Results[index].Customer = result
		response.Accepted++
	}
	return response, nil
}

func (s *CustomerService) authorize(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, error) {
	authenticatedCtx, _, membership, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate customer request: %w", err)
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceCustomers, permission) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceCustomers, permission,
			fmt.Sprintf("Insufficient permissions: %s access to customers required", permission),
		)
	}
	return authenticatedCtx, nil
}

func (s *CustomerService) upsertValidated(ctx context.Context, workspace *domain.Workspace, idempotencyKey string, input domain.CustomerUpsertInput) (*domain.CustomerMutationResult, error) {
	if workspace == nil {
		return nil, errors.New("workspace is required")
	}
	payloadHash, err := input.CanonicalPayloadHash()
	if err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	return s.repository.Upsert(ctx, domain.CustomerUpsertCommand{
		WorkspaceID: workspace.ID, WorkspaceSequence: workspace.Sequence,
		IdempotencyKey: idempotencyKey, PayloadHash: payloadHash, Input: input,
	})
}

func customerBatchError(err error) *domain.CustomerBatchItemError {
	var validation domain.ValidationError
	var identity *domain.ErrCustomerIdentityConflict
	var external *domain.ErrCustomerExternalIDConflict
	var idempotency *domain.ErrCustomerIdempotencyConflict
	var notFound *domain.ErrCustomerNotFound
	switch {
	case errors.As(err, &validation):
		return &domain.CustomerBatchItemError{Code: "validation_error", Message: validation.Error()}
	case errors.As(err, &identity):
		return &domain.CustomerBatchItemError{Code: "identity_conflict", Message: identity.Error()}
	case errors.As(err, &external):
		return &domain.CustomerBatchItemError{Code: "external_id_conflict", Message: external.Error()}
	case errors.As(err, &idempotency):
		return &domain.CustomerBatchItemError{Code: "idempotency_conflict", Message: idempotency.Error()}
	case errors.As(err, &notFound):
		return &domain.CustomerBatchItemError{Code: "customer_not_found", Message: notFound.Error()}
	default:
		return &domain.CustomerBatchItemError{Code: "internal_error", Message: "customer mutation failed"}
	}
}
