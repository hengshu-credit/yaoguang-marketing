package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contactEndpointRepositoryStub struct {
	domain.ContactEndpointRepository
	workspaceID string
	email       string
	channel     string
	endpoints   []*domain.ContactEndpoint
}

func (s *contactEndpointRepositoryStub) ListActiveByEmail(
	_ context.Context, workspaceID, email, channel string,
) ([]*domain.ContactEndpoint, error) {
	s.workspaceID, s.email, s.channel = workspaceID, email, channel
	return s.endpoints, nil
}

func TestContactEndpointServiceListsSafeMetadata(t *testing.T) {
	auth := &ingestAuthStub{permissions: domain.UserPermissions{
		domain.PermissionResourceContacts: {Read: true},
	}}
	repo := &contactEndpointRepositoryStub{endpoints: []*domain.ContactEndpoint{{
		EndpointID: "device-1", Email: "user@example.com", Channel: domain.ChannelPush,
		Provider: domain.PushProviderFCM, Platform: domain.EndpointPlatformAndroid,
		Address: "secret-token", Enabled: true,
	}}}
	service, err := NewContactEndpointService(auth, repo)
	require.NoError(t, err)

	endpoints, err := service.List(context.Background(), &domain.ListContactEndpointsRequest{
		WorkspaceID: "workspace-1", Email: "USER@example.com",
	})
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	assert.Equal(t, "user@example.com", repo.email)
	assert.Equal(t, domain.ChannelPush, repo.channel)
	assert.Empty(t, endpoints[0].Address)
	assert.Equal(t, "secret-token", repo.endpoints[0].Address, "repository result must not be mutated")
	encoded, err := endpoints[0].MarshalPublicJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret-token")
}
