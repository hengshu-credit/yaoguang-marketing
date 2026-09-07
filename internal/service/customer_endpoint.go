package service

import (
	"context"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (s *ContactEndpointService) SaveCustomerEndpoint(ctx context.Context, request *domain.CustomerEndpointRequest) error {
	if err := request.Validate(); err != nil {
		return domain.NewValidationError(err.Error())
	}
	ctx, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		return err
	}
	if !membership.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeWrite, "Contact write permission is required")
	}
	repo, ok := s.repository.(interface {
		SaveCustomerEndpoint(context.Context, *domain.CustomerEndpointRequest) error
	})
	if !ok {
		return errors.New("customer endpoint storage unavailable")
	}
	request.ActorID = membership.UserID
	return repo.SaveCustomerEndpoint(ctx, request)
}
