package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
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

type ingestContactRepoStub struct {
	domain.ContactRepository
	bulkCalls   int
	bulkInputs  [][]*domain.Contact
	bulkResults []domain.BulkUpsertResult
	err         error
}

func (s *ingestContactRepoStub) BulkUpsertContacts(_ context.Context, _ string, contacts []*domain.Contact) ([]domain.BulkUpsertResult, error) {
	s.bulkCalls++
	s.bulkInputs = append(s.bulkInputs, contacts)
	if s.err != nil {
		return nil, s.err
	}
	if s.bulkResults != nil {
		return s.bulkResults, nil
	}
	results := make([]domain.BulkUpsertResult, len(contacts))
	for i, contact := range contacts {
		results[i] = domain.BulkUpsertResult{Email: contact.Email, IsNew: true}
	}
	return results, nil
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
	contacts := &ingestContactRepoStub{}
	profiles := &ingestProfileRepoStub{}
	lists := &ingestListRepoStub{}
	events := &ingestEventRepoStub{}
	service, err := NewIngestService(auth, contacts, profiles, lists, events, 500, 2)
	require.NoError(t, err)

	status := "active"
	request := &domain.IngestBatchRequest{
		WorkspaceID: "workspace-1",
		Contacts: []domain.IngestContact{{
			ID: "contact-1", Contact: json.RawMessage(`{"email":"USER@example.com"}`),
			Status: &status, Attributes: map[string]interface{}{"plan": "pro"},
			Tags:            &domain.TagMutation{Operation: domain.TagOperationSet, Values: []string{"paid"}},
			ListMemberships: []domain.IngestListMembership{{ListID: "news", Status: domain.ContactListStatusActive}},
		}},
		Events: []domain.IngestEvent{{
			ID: "event-1", Email: "other@example.com", EventName: "order.completed", ExternalID: "order-1",
		}},
	}

	response, err := service.IngestBatch(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, 1, auth.calls)
	assert.Equal(t, 1, contacts.bulkCalls)
	assert.Equal(t, "user@example.com", contacts.bulkInputs[0][0].Email)
	assert.Equal(t, []string{"other@example.com"}, profiles.ensured)
	assert.Equal(t, []string{"user@example.com"}, profiles.profiles)
	assert.Equal(t, []string{"user@example.com"}, profiles.tagged)
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
	contacts := &ingestContactRepoStub{}
	profiles := &ingestProfileRepoStub{}
	service, err := NewIngestService(auth, contacts, profiles, &ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1)
	require.NoError(t, err)

	request := &domain.IngestBatchRequest{
		WorkspaceID: "workspace-1",
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

func TestIngestServiceRequiresListsWriteForMembershipChanges(t *testing.T) {
	auth := &ingestAuthStub{permissions: ingestWritePermissions(false)}
	service, err := NewIngestService(auth, &ingestContactRepoStub{}, &ingestProfileRepoStub{}, &ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1)
	require.NoError(t, err)

	request := &domain.IngestBatchRequest{
		WorkspaceID: "workspace-1",
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
		&ingestContactRepoStub{}, &ingestProfileRepoStub{}, &ingestListRepoStub{}, &ingestEventRepoStub{}, 500, 1,
	)
	require.NoError(t, err)
	service.slots <- struct{}{}
	defer func() { <-service.slots }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = service.IngestBatch(ctx, &domain.IngestBatchRequest{
		WorkspaceID: "workspace-1",
		Contacts:    []domain.IngestContact{{ID: "one", Contact: json.RawMessage(`{"email":"user@example.com"}`)}},
	})
	assert.ErrorIs(t, err, ErrIngestBusy)
}
