package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

var ErrIngestBusy = errors.New("ingest capacity is full")

type IngestService struct {
	auth         domain.AuthService
	contacts     domain.ContactRepository
	profiles     domain.AudienceProfileRepository
	endpoints    domain.ContactEndpointRepository
	lists        domain.ContactListRepository
	events       domain.CustomEventRepository
	maxBatchSize int
	slots        chan struct{}
	now          func() time.Time
}

func NewIngestService(
	auth domain.AuthService,
	contacts domain.ContactRepository,
	profiles domain.AudienceProfileRepository,
	endpoints domain.ContactEndpointRepository,
	lists domain.ContactListRepository,
	events domain.CustomEventRepository,
	maxBatchSize int,
	maxInFlight int,
) (*IngestService, error) {
	if auth == nil || contacts == nil || profiles == nil || endpoints == nil || lists == nil || events == nil {
		return nil, errors.New("ingest dependencies are required")
	}
	if maxBatchSize <= 0 || maxInFlight <= 0 {
		return nil, errors.New("ingest batch and concurrency limits must be positive")
	}
	return &IngestService{
		auth: auth, contacts: contacts, profiles: profiles, endpoints: endpoints, lists: lists, events: events,
		maxBatchSize: maxBatchSize, slots: make(chan struct{}, maxInFlight),
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *IngestService) IngestBatch(ctx context.Context, request *domain.IngestBatchRequest) (*domain.IngestBatchResponse, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	if err := request.Validate(s.maxBatchSize); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, ErrIngestBusy
	}

	authenticatedCtx, _, userWorkspace, err := s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate ingest request: %w", err)
	}
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts, domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required for ingest",
		)
	}
	if requestChangesLists(request) && !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists, domain.PermissionTypeWrite,
			"Insufficient permissions: write access to lists required for membership ingest",
		)
	}

	response := &domain.IngestBatchResponse{
		Results: make([]domain.IngestItemResult, len(request.Contacts)+len(request.Events)),
	}
	s.processContacts(authenticatedCtx, request, response)
	s.processEvents(authenticatedCtx, request, response)
	for _, result := range response.Results {
		if result.Status == "accepted" {
			response.Accepted++
		} else {
			response.Failed++
		}
	}
	return response, nil
}

func requestChangesLists(request *domain.IngestBatchRequest) bool {
	for i := range request.Contacts {
		if len(request.Contacts[i].ListMemberships) > 0 {
			return true
		}
	}
	return false
}

func (s *IngestService) processContacts(
	ctx context.Context,
	request *domain.IngestBatchRequest,
	response *domain.IngestBatchResponse,
) {
	validContacts := make([]*domain.Contact, 0, len(request.Contacts))
	validIndices := make([]int, 0, len(request.Contacts))
	lastByEmail := make(map[string]int, len(request.Contacts))

	for index := range request.Contacts {
		record := &request.Contacts[index]
		response.Results[index] = domain.IngestItemResult{ID: record.ID, Type: "contact", Status: "error"}
		contact, err := record.Validate()
		if err != nil {
			response.Results[index].Error = err.Error()
			continue
		}
		response.Results[index].Key = contact.Email
		validContacts = append(validContacts, contact)
		validIndices = append(validIndices, index)
		lastByEmail[contact.Email] = index
	}

	deduplicated := validContacts[:0]
	deduplicatedIndices := validIndices[:0]
	for index, contact := range validContacts {
		resultIndex := validIndices[index]
		if lastByEmail[contact.Email] != resultIndex {
			response.Results[resultIndex].Error = "duplicate contact email in batch; later record wins"
			continue
		}
		deduplicated = append(deduplicated, contact)
		deduplicatedIndices = append(deduplicatedIndices, resultIndex)
	}
	validContacts, validIndices = deduplicated, deduplicatedIndices

	for start := 0; start < len(validContacts); start += domain.BulkImportChunkSize {
		end := min(start+domain.BulkImportChunkSize, len(validContacts))
		results, err := s.contacts.BulkUpsertContacts(ctx, request.WorkspaceID, validContacts[start:end])
		if err != nil {
			for _, resultIndex := range validIndices[start:end] {
				response.Results[resultIndex].Error = err.Error()
				response.Results[resultIndex].Retryable = true
			}
			continue
		}
		actionByEmail := make(map[string]string, len(results))
		for _, result := range results {
			action := domain.UpsertContactOperationUpdate
			if result.IsNew {
				action = domain.UpsertContactOperationCreate
			}
			actionByEmail[result.Email] = action
		}
		for offset, contact := range validContacts[start:end] {
			resultIndex := validIndices[start+offset]
			action, ok := actionByEmail[contact.Email]
			if !ok {
				response.Results[resultIndex].Error = "contact upsert returned no result"
				response.Results[resultIndex].Retryable = true
				continue
			}
			response.Results[resultIndex].Action = action
			if err := s.applyContactExtensions(ctx, request.WorkspaceID, contact.Email, &request.Contacts[resultIndex]); err != nil {
				response.Results[resultIndex].Error = err.Error()
				response.Results[resultIndex].Retryable = true
				continue
			}
			response.Results[resultIndex].Status = "accepted"
			response.Results[resultIndex].Error = ""
		}
	}
}

func (s *IngestService) applyContactExtensions(
	ctx context.Context,
	workspaceID string,
	email string,
	record *domain.IngestContact,
) error {
	if record.Status != nil || record.Attributes != nil {
		if err := s.profiles.UpsertProfile(ctx, workspaceID, email, record.Status, record.Attributes); err != nil {
			return fmt.Errorf("update profile: %w", err)
		}
	}
	if record.Tags != nil {
		if _, err := s.profiles.ApplyTags(ctx, workspaceID, email, record.Tags.Operation, record.Tags.Values); err != nil {
			return fmt.Errorf("update tags: %w", err)
		}
	}
	for _, mutation := range record.Endpoints {
		endpoint, err := mutation.Validate()
		if err != nil {
			return fmt.Errorf("update endpoint %s: %w", mutation.EndpointID, err)
		}
		if !endpoint.Enabled {
			if err := s.endpoints.Disable(ctx, workspaceID, email, endpoint.EndpointID); err != nil {
				return fmt.Errorf("disable endpoint %s: %w", endpoint.EndpointID, err)
			}
			continue
		}
		if err := s.endpoints.Upsert(ctx, workspaceID, email, endpoint); err != nil {
			return fmt.Errorf("upsert endpoint %s: %w", endpoint.EndpointID, err)
		}
	}
	for _, membership := range record.ListMemberships {
		if err := s.lists.AddContactToList(ctx, workspaceID, &domain.ContactList{
			Email: email, ListID: membership.ListID, Status: membership.Status,
		}); err != nil {
			return fmt.Errorf("update list %s: %w", membership.ListID, err)
		}
	}
	return nil
}

func (s *IngestService) processEvents(
	ctx context.Context,
	request *domain.IngestBatchRequest,
	response *domain.IngestBatchResponse,
) {
	offset := len(request.Contacts)
	now := s.now().UTC()
	validEvents := make([]*domain.CustomEvent, 0, len(request.Events))
	validIndices := make([]int, 0, len(request.Events))
	lastByKey := make(map[string]int, len(request.Events))

	for index := range request.Events {
		record := &request.Events[index]
		resultIndex := offset + index
		response.Results[resultIndex] = domain.IngestItemResult{ID: record.ID, Type: "event", Status: "error"}
		event, err := record.Validate(now)
		if err != nil {
			response.Results[resultIndex].Error = err.Error()
			continue
		}
		key := event.EventName + ":" + event.ExternalID
		response.Results[resultIndex].Key = key
		validEvents = append(validEvents, event)
		validIndices = append(validIndices, resultIndex)
		lastByKey[key] = resultIndex
	}

	deduplicated := validEvents[:0]
	deduplicatedIndices := validIndices[:0]
	for index, event := range validEvents {
		resultIndex := validIndices[index]
		key := event.EventName + ":" + event.ExternalID
		if lastByKey[key] != resultIndex {
			response.Results[resultIndex].Error = "duplicate event key in batch; later record wins"
			continue
		}
		deduplicated = append(deduplicated, event)
		deduplicatedIndices = append(deduplicatedIndices, resultIndex)
	}
	validEvents, validIndices = deduplicated, deduplicatedIndices
	if len(validEvents) == 0 {
		return
	}

	emailSet := make(map[string]struct{}, len(validEvents))
	for _, event := range validEvents {
		emailSet[event.Email] = struct{}{}
	}
	emails := make([]string, 0, len(emailSet))
	for email := range emailSet {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	if err := s.profiles.EnsureContacts(ctx, request.WorkspaceID, emails); err != nil {
		for _, resultIndex := range validIndices {
			response.Results[resultIndex].Error = fmt.Sprintf("ensure event contact: %v", err)
			response.Results[resultIndex].Retryable = true
		}
		return
	}

	const eventChunkSize = 100
	for start := 0; start < len(validEvents); start += eventChunkSize {
		end := min(start+eventChunkSize, len(validEvents))
		err := s.events.BatchUpsert(ctx, request.WorkspaceID, validEvents[start:end])
		for _, resultIndex := range validIndices[start:end] {
			if err != nil {
				response.Results[resultIndex].Error = err.Error()
				response.Results[resultIndex].Retryable = true
				continue
			}
			response.Results[resultIndex].Status = "accepted"
			response.Results[resultIndex].Action = "upsert"
		}
	}
}
