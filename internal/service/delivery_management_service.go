package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type DeliveryManagementService struct {
	repository domain.DeliveryManagementRepository
	auth       domain.AuthService
}

func NewDeliveryManagementService(repository domain.DeliveryManagementRepository, auth domain.AuthService) (*DeliveryManagementService, error) {
	if repository == nil || auth == nil {
		return nil, errors.New("delivery management dependencies are required")
	}
	return &DeliveryManagementService{repository: repository, auth: auth}, nil
}

func (s *DeliveryManagementService) authorize(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, *domain.User, error) {
	authorized, user, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("authenticate delivery management request: %w", err)
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceMessageHistory, permission) {
		return nil, nil, domain.NewPermissionError(domain.PermissionResourceMessageHistory, permission, "Message history permission is required for delivery management")
	}
	return authorized, user, nil
}

func (s *DeliveryManagementService) List(ctx context.Context, request *domain.DeliveryListRequest) ([]domain.DeliveryIntent, int, error) {
	if err := request.Validate(); err != nil {
		return nil, 0, domain.NewValidationError(err.Error())
	}
	authorized, _, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.ListDeliveries(authorized, request.WorkspaceID, request.Status, request.Limit, request.Offset)
}

func (s *DeliveryManagementService) Get(ctx context.Context, request *domain.DeliveryGetRequest) (*domain.DeliveryDetail, error) {
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authorized, _, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.GetDelivery(authorized, request.WorkspaceID, request.IntentID)
}

func (s *DeliveryManagementService) Reconcile(ctx context.Context, request *domain.DeliveryReconcileRequest) error {
	if err := request.Validate(); err != nil {
		return domain.NewValidationError(err.Error())
	}
	authorized, _, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	return s.repository.RequestDeliveryReconciliation(authorized, request.WorkspaceID, request.IntentID, "manual reconciliation requested")
}

func (s *DeliveryManagementService) ResolveUnknown(ctx context.Context, request *domain.DeliveryResolveUnknownRequest) error {
	if err := request.Validate(); err != nil {
		return domain.NewValidationError(err.Error())
	}
	authorized, user, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	actorID := "system"
	if user != nil && user.ID != "" {
		actorID = user.ID
	}
	return s.repository.ResolveUnknownDelivery(authorized, request.WorkspaceID, request.IntentID, request.Action, actorID, request.Reason)
}
