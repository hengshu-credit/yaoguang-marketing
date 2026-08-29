package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	TagOperationSet    = "set"
	TagOperationAdd    = "add"
	TagOperationRemove = "remove"
)

type TagMutation struct {
	Operation string   `json:"operation"`
	Values    []string `json:"values"`
}

type IngestListMembership struct {
	ListID string            `json:"list_id"`
	Status ContactListStatus `json:"status"`
}

// IngestContact combines the stable contact fields with the external system's
// application-owned profile, tags and marketing-list state.
type IngestContact struct {
	ID              string                    `json:"id"`
	Contact         json.RawMessage           `json:"contact"`
	Status          *string                   `json:"status,omitempty"`
	Attributes      map[string]interface{}    `json:"attributes,omitempty"`
	Tags            *TagMutation              `json:"tags,omitempty"`
	ListMemberships []IngestListMembership    `json:"list_memberships,omitempty"`
	Endpoints       []ContactEndpointMutation `json:"endpoints,omitempty"`
}

func (r *IngestContact) Validate() (*Contact, error) {
	if strings.TrimSpace(r.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	contact, err := FromJSON([]byte(r.Contact))
	if err != nil {
		return nil, fmt.Errorf("invalid contact: %w", err)
	}
	if r.Status != nil {
		status := strings.TrimSpace(*r.Status)
		if status == "" || utf8.RuneCountInString(status) > 64 {
			return nil, fmt.Errorf("status must contain 1 to 64 characters")
		}
		*r.Status = status
	}
	if r.Tags != nil {
		switch r.Tags.Operation {
		case TagOperationSet, TagOperationAdd, TagOperationRemove:
		default:
			return nil, fmt.Errorf("tag operation must be set, add, or remove")
		}
		tags := make(map[string]struct{}, len(r.Tags.Values))
		for _, raw := range r.Tags.Values {
			tag := strings.TrimSpace(raw)
			if tag == "" || utf8.RuneCountInString(tag) > 64 {
				return nil, fmt.Errorf("tags must contain 1 to 64 characters")
			}
			tags[tag] = struct{}{}
		}
		if r.Tags.Operation != TagOperationSet && len(tags) == 0 {
			return nil, fmt.Errorf("tag operation %s requires at least one value", r.Tags.Operation)
		}
		r.Tags.Values = make([]string, 0, len(tags))
		for tag := range tags {
			r.Tags.Values = append(r.Tags.Values, tag)
		}
		sort.Strings(r.Tags.Values)
	}
	for index, membership := range r.ListMemberships {
		if strings.TrimSpace(membership.ListID) == "" {
			return nil, fmt.Errorf("list membership at index %d: list_id is required", index)
		}
		if !validIngestListStatus(membership.Status) {
			return nil, fmt.Errorf("list membership at index %d: invalid list status %q", index, membership.Status)
		}
	}
	endpointIDs := make(map[string]struct{}, len(r.Endpoints))
	for index, mutation := range r.Endpoints {
		endpoint, err := mutation.Validate()
		if err != nil {
			return nil, fmt.Errorf("endpoint at index %d: %w", index, err)
		}
		if _, duplicate := endpointIDs[endpoint.EndpointID]; duplicate {
			return nil, fmt.Errorf("endpoint at index %d: duplicate endpoint_id %q", index, endpoint.EndpointID)
		}
		endpointIDs[endpoint.EndpointID] = struct{}{}
	}
	return contact, nil
}

func validIngestListStatus(status ContactListStatus) bool {
	switch status {
	case ContactListStatusActive, ContactListStatusPending, ContactListStatusUnsubscribed,
		ContactListStatusBounced, ContactListStatusComplained:
		return true
	default:
		return false
	}
}

type IngestEvent struct {
	ID            string                 `json:"id"`
	Email         string                 `json:"email"`
	EventName     string                 `json:"event_name"`
	ExternalID    string                 `json:"external_id"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
	OccurredAt    *time.Time             `json:"occurred_at,omitempty"`
	IntegrationID *string                `json:"integration_id,omitempty"`
	GoalName      *string                `json:"goal_name,omitempty"`
	GoalType      *string                `json:"goal_type,omitempty"`
	GoalValue     *float64               `json:"goal_value,omitempty"`
}

func (r *IngestEvent) Validate(now time.Time) (*CustomEvent, error) {
	if strings.TrimSpace(r.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	occurredAt := now.UTC()
	if r.OccurredAt != nil {
		occurredAt = r.OccurredAt.UTC()
	}
	contact := &Contact{Email: r.Email}
	if err := contact.Validate(); err != nil {
		return nil, err
	}
	event := &CustomEvent{
		ExternalID: r.ExternalID, Email: contact.Email, EventName: r.EventName,
		Properties: r.Properties, OccurredAt: occurredAt, Source: "api",
		IntegrationID: r.IntegrationID, GoalName: r.GoalName, GoalType: r.GoalType,
		GoalValue: r.GoalValue, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return event, nil
}

type IngestBatchRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	Contacts    []IngestContact `json:"contacts,omitempty"`
	Events      []IngestEvent   `json:"events,omitempty"`
}

func (r *IngestBatchRequest) Validate(maxRecords int) error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return fmt.Errorf("workspace_id is required")
	}
	total := len(r.Contacts) + len(r.Events)
	if total == 0 {
		return fmt.Errorf("at least one contact or event is required")
	}
	if maxRecords <= 0 {
		return fmt.Errorf("max records must be positive")
	}
	if total > maxRecords {
		return fmt.Errorf("batch cannot contain more than %d records", maxRecords)
	}
	return nil
}

type IngestItemResult struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Key       string `json:"key,omitempty"`
	Status    string `json:"status"`
	Action    string `json:"action,omitempty"`
	Error     string `json:"error,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

type IngestBatchResponse struct {
	Accepted int                `json:"accepted"`
	Failed   int                `json:"failed"`
	Results  []IngestItemResult `json:"results"`
}

// AudienceProfileRepository owns the application-facing profile extension and
// tag set. Contact and consent storage remain in their existing repositories.
type AudienceProfileRepository interface {
	EnsureContacts(ctx context.Context, workspaceID string, emails []string) error
	UpsertProfile(ctx context.Context, workspaceID, email string, status *string, attributes map[string]interface{}) error
	ApplyTags(ctx context.Context, workspaceID, email, operation string, tags []string) ([]string, error)
	GetProfiles(ctx context.Context, workspaceID string, emails []string) (map[string]*AudienceProfile, error)
}

type IngestService interface {
	IngestBatch(ctx context.Context, request *IngestBatchRequest) (*IngestBatchResponse, error)
}
