package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ContactEndpointService struct {
	auth       domain.AuthService
	repository domain.ContactEndpointRepository
}

func NewContactEndpointService(
	auth domain.AuthService,
	repository domain.ContactEndpointRepository,
) (*ContactEndpointService, error) {
	if auth == nil || repository == nil {
		return nil, errors.New("contact endpoint service dependencies are required")
	}
	return &ContactEndpointService{auth: auth, repository: repository}, nil
}

func (s *ContactEndpointService) List(
	ctx context.Context,
	request *domain.ListContactEndpointsRequest,
) ([]*domain.ContactEndpoint, error) {
	if err := request.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authenticatedCtx, _, userWorkspace, err := s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate endpoint list request: %w", err)
	}
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts, domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required for endpoint list",
		)
	}
	endpoints, err := s.repository.ListActiveByEmail(
		authenticatedCtx, request.WorkspaceID, request.Email, request.Channel,
	)
	if err != nil {
		return nil, err
	}
	metadata := make([]*domain.ContactEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint == nil {
			continue
		}
		publicEndpoint := *endpoint
		publicEndpoint.Address = ""
		metadata = append(metadata, &publicEndpoint)
	}
	return metadata, nil
}
