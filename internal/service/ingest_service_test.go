package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ingestAuthStub struct {
	domain.AuthService
	calls       int
	permissions domain.UserPermissions
	err         error
}

func (s *ingestAuthStub) AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	s.calls++
	if s.err != nil {
		return ctx, nil, nil, s.err
	}
	return ctx, &domain.User{ID: "api-key"}, &domain.UserWorkspace{
		UserID: "api-key", WorkspaceID: workspaceID, Role: "member", Permissions: s.permissions,
	}, nil
}

type ingestCustomerServiceStub struct {
	domain.CustomerService
	batchCalls  int
	batchInputs []*domain.CustomerBatchUpsertRequest
	err         error
}

func (s *ingestCustomerServiceStub) UpsertCustomerBatch(_ context.Context, request *domain.CustomerBatchUpsertRequest) (*domain.CustomerBatchUpsertResponse, error) {
	s.batchCalls++
	s.batchInputs = append(s.batchInputs, request)
	if s.err != nil {
		return nil, s.err
	}
	response := &domain.CustomerBatchUpsertResponse{Results: make([]domain.CustomerBatchItemResult, len(request.Items))}
	for index := range request.Items {
		response.Results[index] = domain.CustomerBatchItemResult{
			Index: index, Status: "accepted", Customer: &domain.CustomerMutationResult{Action: "created"},
		}
		response.Accepted++
	}
	return response, nil
}

func newIngestCustomerAuthority(t *testing.T, customers domain.CustomerService) *LegacyContactAdapter {
	t.Helper()
	adapter, err := NewLegacyContactAdapter(customers, 10_000)
	require.NoError(t, err)
	return adapter
}

type ingestProfileRepoStub struct {
	domain.AudienceProfileRepository
	ensured    []string
	profiles   []string
	tagged     []string
	profileErr error
}

func (s *ingestProfileRepoStub) EnsureContacts(_ context.Context, _ string, emails []string) error {
	s.ensured = append(s.ensured, emails...)
	return nil
}

func (s *ingestProfileRepoStub) UpsertProfile(_ context.Context, _, email string, _ *string, _ map[string]interface{}) error {
	s.profiles = append(s.profiles, email)
	return s.profileErr
}

func (s *ingestProfileRepoStub) ApplyTags(_ context.Context, _, email, _ string, _ []string) ([]string, error) {
	s.tagged = append(s.tagged, email)
	return []string{"paid"}, nil
}

type ingestListRepoStub struct {
	domain.ContactListRepository
	memberships []domain.ContactList
}

func (s *ingestListRepoStub) AddContactToList(_ context.Context, _ string, membership *domain.ContactList) error {
	s.memberships = append(s.memberships, *membership)
	return nil
}

type ingestEventRepoStub struct {
	domain.CustomEventRepository
	events []*domain.CustomEvent
	err    error
}

type ingestEndpointRepoStub struct {
	domain.ContactEndpointRepository
	upserted []string
	disabled []string
	err      error
}

func (s *ingestEndpointRepoStub) Upsert(_ context.Context, _, email string, endpoint *domain.ContactEndpoint) error {
	s.upserted = append(s.upserted, email+":"+endpoint.EndpointID)
	return s.err
}

func (s *ingestEndpointRepoStub) Disable(_ context.Context, _, email, endpointID string) error {
	s.disabled = append(s.disabled, email+":"+endpointID)
	return s.err
}

func (s *ingestEventRepoStub) BatchUpsert(_ context.Context, _ string, events []*domain.CustomEvent) error {
	s.events = append(s.events, events...)
	return s.err
}

func ingestWritePermissions(withLists bool) domain.UserPermissions {
	permissions := domain.UserPermissions{
		domain.PermissionResourceContacts: {Write: true},
	}
	if withLists {
		permissions[domain.PermissionResourceLists] = domain.ResourcePermissions{Write: true}
	}
	return permissions
}

func TestIngestServiceBatchAuthenticatesOnceAndAppliesAllMutationTypes(t *testing.T) {
	auth := &ingestAuthStub{permissions: ingestWritePermissions(true)}
	customers := &ingestCustomerServiceStub{}
	profiles := &ingestProfileRepoStub{}
	lists := &ingestListRepoStub{}
	events := &ingestEventRepoStub{}
	endpoints := &ingestEndpointRepoStub{}
	service, err := NewIngestService(auth, newIngestCustomerAuthority(t, customers), profiles, endpoints, lists, events, 500, 2)
	require.NoError(t, err)

	status := "active"
	request := &domain.IngestBatchRequest{
		WorkspaceID: "workspace1",
		Contacts: []domain.IngestContact{{
			ID: "contact-1", Contact: json.RawMessage(`{"email":"USER@example.com"}`),
			Status: &status, Attributes: map[string]interface{}{"plan": "pro"},
			Tags:            &domain.TagMutation{Operation: domain.TagOperationSet, Values: []string{"paid"}},
			ListMemberships: []domain.IngestListMembership{{ListID: "news", Status: domain.ContactListStatusActive}},
			Endpoints: []domain.ContactEndpointMutation{{
				Operation: domain.EndpointOperationUpsert, EndpointID: "device-1", Channel: domain.ChannelPush,
				Provider: domain.PushProviderFCM, Platform: domain.EndpointPlatformAndroid, Address: "token-1",
			}},
		}},
		Events: []domain.IngestEvent{{
			ID: "event-1", Email: "other@example.com", EventName: "order.completed", ExternalID: "order-1",
		}},
	}

	response, err := service.IngestBatch(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, 1, auth.calls)
	assert.Equal(t, 1, customers.batchCalls)
	assert.Equal(t, "user@example.com", customers.batchInputs[0].Items[0].Customer.Identities[0].Value)
	require.NotNil(t, customers.batchInputs[0].Items[0].Customer.Profile)
	require.NotNil(t, customers.batchInputs[0].Items[0].Customer.Tags)
	require.NotNil(t, customers.batchInputs[0].Items[0].Customer.ListMemberships)
	assert.Equal(t, []string{"other@example.com"}, profiles.ensured)
	assert.Equal(t, []string{"user@example.com"}, profiles.profiles)
	assert.Equal(t, []string{"user@example.com"}, profiles.tagged)
	assert.Equal(t, []string{"user@example.com:device-1"}, endpoints.upserted)
	require.Len(t, lists.memberships, 1)
	assert.Equal(t, domain.ContactListStatusActive, lists.memberships[0].Status)
	require.Len(t, events.events, 1)
	assert.Equal(t, "order.completed", events.events[0].EventName)
	assert.Equal(t, 2, response.Accepted)
	assert.Zero(t, response.Failed)
	assert.Equal(t, []string{"contact-1", "event-1"}, []string{response.Results[0].ID, response.Results[1].ID})
}

func TestIngestServiceBatchReturnsPerItemValidationErrors(t *testing.T) {
	auth := &ingestAuthStub{permissions: ingestWritePermissions(false)}
	customers := &ingestCustomerServiceStub{}
	profiles := &ingestProfileRepoStub{}
	service, err := NewIngestService(auth, newIngestCustomerAuthority(t, customers), profiles, &ingestEndpointRepoStub{}, &ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1)
	require.NoError(t, err)

	request := &domain.IngestBatchRequest{
		WorkspaceID: "workspace1",
		Contacts: []domain.IngestContact{
			{ID: "bad", Contact: json.RawMessage(`{"email":"not-an-email"}`)},
			{ID: "good", Contact: json.RawMessage(`{"email":"good@example.com"}`)},
		},
	}

	response, err := service.IngestBatch(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Accepted)
	assert.Equal(t, 1, response.Failed)
	assert.Equal(t, "error", response.Results[0].Status)
	assert.False(t, response.Results[0].Retryable)
	assert.Equal(t, "accepted", response.Results[1].Status)
}

func TestIngestServiceNormalizesEndpointDisableOperation(t *testing.T) {
	auth := &ingestAuthStub{permissions: ingestWritePermissions(false)}
	endpoints := &ingestEndpointRepoStub{}
	service, err := NewIngestService(
		auth, newIngestCustomerAuthority(t, &ingestCustomerServiceStub{}), &ingestProfileRepoStub{}, endpoints,
		&ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1,
	)
	require.NoError(t, err)

	response, err := service.IngestBatch(context.Background(), &domain.IngestBatchRequest{
		WorkspaceID: "workspace1",
		Contacts: []domain.IngestContact{{
			ID: "contact-1", Contact: json.RawMessage(`{"email":"user@example.com"}`),
			Endpoints: []domain.ContactEndpointMutation{{Operation: " DISABLE ", EndpointID: "device-1"}},
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Accepted)
	assert.Equal(t, []string{"user@example.com:device-1"}, endpoints.disabled)
	assert.Empty(t, endpoints.upserted)
}

func TestIngestServiceRequiresListsWriteForMembershipChanges(t *testing.T) {
	auth := &ingestAuthStub{permissions: ingestWritePermissions(false)}
	service, err := NewIngestService(auth, newIngestCustomerAuthority(t, &ingestCustomerServiceStub{}), &ingestProfileRepoStub{}, &ingestEndpointRepoStub{}, &ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1)
	require.NoError(t, err)

	request := &domain.IngestBatchRequest{
		WorkspaceID: "workspace1",
		Contacts: []domain.IngestContact{{
			ID: "contact-1", Contact: json.RawMessage(`{"email":"user@example.com"}`),
			ListMemberships: []domain.IngestListMembership{{ListID: "news", Status: domain.ContactListStatusActive}},
		}},
	}

	_, err = service.IngestBatch(context.Background(), request)
	var permissionError *domain.PermissionError
	assert.True(t, errors.As(err, &permissionError))
	assert.Equal(t, domain.PermissionResourceLists, permissionError.Resource)
}

func TestIngestServiceRejectsImmediatelyWhenCapacityIsFull(t *testing.T) {
	service, err := NewIngestService(
		&ingestAuthStub{permissions: ingestWritePermissions(false)},
		newIngestCustomerAuthority(t, &ingestCustomerServiceStub{}), &ingestProfileRepoStub{}, &ingestEndpointRepoStub{}, &ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1,
	)
	require.NoError(t, err)
	service.slots <- struct{}{}
	defer func() { <-service.slots }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = service.IngestBatch(ctx, &domain.IngestBatchRequest{
		WorkspaceID: "workspace1",
		Contacts:    []domain.IngestContact{{ID: "one", Contact: json.RawMessage(`{"email":"user@example.com"}`)}},
	})
	assert.ErrorIs(t, err, ErrIngestBusy)
}
