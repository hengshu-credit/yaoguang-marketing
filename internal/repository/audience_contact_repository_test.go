package repository

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type audienceContactBaseStub struct {
	domain.ContactRepository
	contacts []*domain.Contact
}

func (s *audienceContactBaseStub) GetContacts(_ context.Context, _ *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
	return &domain.GetContactsResponse{Contacts: s.contacts}, nil
}

type audienceProfileReadStub struct {
	domain.AudienceProfileRepository
	calls  int
	emails []string
}

func (s *audienceProfileReadStub) GetProfiles(_ context.Context, _ string, emails []string) (map[string]*domain.AudienceProfile, error) {
	s.calls++
	s.emails = append([]string(nil), emails...)
	status := "active"
	return map[string]*domain.AudienceProfile{
		"a@example.com": {Status: &status, Attributes: map[string]interface{}{"plan": "pro"}, Tags: []string{"paid"}},
	}, nil
}

func TestAudienceContactRepositoryHydratesAContactPageInOneQuery(t *testing.T) {
	base := &audienceContactBaseStub{contacts: []*domain.Contact{
		{Email: "a@example.com"}, {Email: "b@example.com"},
	}}
	profiles := &audienceProfileReadStub{}
	repo := NewAudienceContactRepository(base, profiles)

	response, err := repo.GetContacts(context.Background(), &domain.GetContactsRequest{WorkspaceID: "workspace-1"})
	require.NoError(t, err)
	assert.Equal(t, 1, profiles.calls)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, profiles.emails)
	require.NotNil(t, response.Contacts[0].Profile)
	assert.Equal(t, "active", *response.Contacts[0].Profile.Status)
	assert.Nil(t, response.Contacts[1].Profile)
}
