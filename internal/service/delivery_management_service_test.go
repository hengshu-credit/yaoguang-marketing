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

type deliveryManagementRepositoryStub struct {
	action domain.DeliveryResolutionAction
	actor  string
	reason string
}

func (r *deliveryManagementRepositoryStub) ListDeliveries(context.Context, domain.DeliveryListRequest) ([]domain.DeliveryIntent, int, error) {
	return []domain.DeliveryIntent{{ID: "intent-1"}}, 1, nil
}
func (r *deliveryManagementRepositoryStub) GetDelivery(context.Context, string, string) (*domain.DeliveryDetail, error) {
	return &domain.DeliveryDetail{Intent: domain.DeliveryIntent{ID: "intent-1"}}, nil
}
func (r *deliveryManagementRepositoryStub) RequestDeliveryReconciliation(context.Context, string, string, string) error {
	return nil
}
func (r *deliveryManagementRepositoryStub) ResolveUnknownDelivery(_ context.Context, _, _ string, action domain.DeliveryResolutionAction, actor, reason string) error {
	r.action, r.actor, r.reason = action, actor, reason
	return nil
}
func (r *deliveryManagementRepositoryStub) GetDeliveryProgress(context.Context, string, domain.DeliverySource, string, string) (domain.DeliveryProgress, error) {
	return domain.DeliveryProgress{}, nil
}

func deliveryMembership(read, write bool) *domain.UserWorkspace {
	return &domain.UserWorkspace{Role: "member", Permissions: domain.UserPermissions{
		domain.PermissionResourceMessageHistory: {Read: read, Write: write},
	}}
}

func TestDeliveryManagementServiceRequiresWriteForUnknownResolution(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	repository := &deliveryManagementRepositoryStub{}
	service, err := NewDeliveryManagementService(repository, auth)
	require.NoError(t, err)
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "workspace-1").
		Return(context.Background(), &domain.User{ID: "user-1"}, deliveryMembership(true, false), nil)

	err = service.ResolveUnknown(context.Background(), &domain.DeliveryResolveUnknownRequest{
		WorkspaceID: "workspace-1", IntentID: "intent-1",
		Action: domain.DeliveryResolutionMarkConfirmed, Reason: "provider dashboard verified delivery",
	})
	var permission *domain.PermissionError
	assert.ErrorAs(t, err, &permission)
}

func TestDeliveryManagementServiceAuditsUnknownResolutionActorAndReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	repository := &deliveryManagementRepositoryStub{}
	service, err := NewDeliveryManagementService(repository, auth)
	require.NoError(t, err)
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "workspace-1").
		Return(context.Background(), &domain.User{ID: "user-1"}, deliveryMembership(true, true), nil)

	err = service.ResolveUnknown(context.Background(), &domain.DeliveryResolveUnknownRequest{
		WorkspaceID: "workspace-1", IntentID: "intent-1",
		Action: domain.DeliveryResolutionRetryVerifiedNotAccepted,
		Reason: "provider dashboard confirms the request was not accepted",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DeliveryResolutionRetryVerifiedNotAccepted, repository.action)
	assert.Equal(t, "user-1", repository.actor)
	assert.Contains(t, repository.reason, "not accepted")
}
