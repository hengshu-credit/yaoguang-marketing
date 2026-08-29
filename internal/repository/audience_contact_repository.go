package repository

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// audienceContactRepository decorates the existing contact repository with a
// set-based read of external profile data. Embedding keeps every legacy write
// and count operation unchanged while the four contact-producing paths expose
// the same profile shape to APIs, automations and broadcast templates.
type audienceContactRepository struct {
	domain.ContactRepository
	profiles domain.AudienceProfileRepository
}

func NewAudienceContactRepository(
	contacts domain.ContactRepository,
	profiles domain.AudienceProfileRepository,
) domain.ContactRepository {
	return &audienceContactRepository{ContactRepository: contacts, profiles: profiles}
}

func (r *audienceContactRepository) GetContactByEmail(ctx context.Context, workspaceID, email string) (*domain.Contact, error) {
	contact, err := r.ContactRepository.GetContactByEmail(ctx, workspaceID, email)
	if err != nil {
		return nil, err
	}
	if err := r.hydrate(ctx, workspaceID, []*domain.Contact{contact}); err != nil {
		return nil, err
	}
	return contact, nil
}

func (r *audienceContactRepository) GetContactByExternalID(ctx context.Context, workspaceID, externalID string) (*domain.Contact, error) {
	contact, err := r.ContactRepository.GetContactByExternalID(ctx, workspaceID, externalID)
	if err != nil {
		return nil, err
	}
	if err := r.hydrate(ctx, workspaceID, []*domain.Contact{contact}); err != nil {
		return nil, err
	}
	return contact, nil
}

func (r *audienceContactRepository) GetContacts(ctx context.Context, request *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
	response, err := r.ContactRepository.GetContacts(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := r.hydrate(ctx, request.WorkspaceID, response.Contacts); err != nil {
		return nil, err
	}
	return response, nil
}

func (r *audienceContactRepository) GetContactsForBroadcast(
	ctx context.Context,
	workspaceID string,
	audience domain.AudienceSettings,
	limit int,
	afterEmail string,
) ([]*domain.ContactWithList, error) {
	contacts, err := r.ContactRepository.GetContactsForBroadcast(ctx, workspaceID, audience, limit, afterEmail)
	if err != nil {
		return nil, err
	}
	plain := make([]*domain.Contact, 0, len(contacts))
	for _, contact := range contacts {
		if contact != nil && contact.Contact != nil {
			plain = append(plain, contact.Contact)
		}
	}
	if err := r.hydrate(ctx, workspaceID, plain); err != nil {
		return nil, err
	}
	return contacts, nil
}

func (r *audienceContactRepository) hydrate(ctx context.Context, workspaceID string, contacts []*domain.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	emails := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		if contact != nil {
			emails = append(emails, contact.Email)
		}
	}
	if len(emails) == 0 {
		return nil
	}
	profiles, err := r.profiles.GetProfiles(ctx, workspaceID, emails)
	if err != nil {
		return fmt.Errorf("hydrate audience profiles: %w", err)
	}
	for _, contact := range contacts {
		if contact != nil {
			contact.Profile = profiles[contact.Email]
		}
	}
	return nil
}
