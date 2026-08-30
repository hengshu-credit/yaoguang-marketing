package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const (
	LegacyContactOperationUpsert = "upsert"
	LegacyContactOperationImport = "import"
	LegacyContactOperationIngest = "ingest"
)

// LegacyContactMutation describes the information carried by an existing
// Contact write endpoint. It deliberately contains no Customer persistence
// concerns: conflict resolution, identity encryption, number generation and
// idempotency storage remain owned by CustomerService and its repository.
type LegacyContactMutation struct {
	Contact         *domain.Contact
	Status          *string
	Attributes      map[string]interface{}
	Tags            *[]string
	ListMemberships *[]domain.CustomerListMembershipInput
}

// LegacyContactAdapter translates the additive legacy Contact contract into
// the Customer authority contract. The max is injected from
// CUSTOMER_SYNC_MAX_BATCH_SIZE so old import/ingest paths cannot bypass the
// configured synchronous safety limit.
type LegacyContactAdapter struct {
	customers    domain.CustomerService
	maxBatchSize int
}

type legacyCustomerAuthorityWriter interface {
	upsertCustomerAuthorized(context.Context, *domain.UpsertCustomerRequest) (*domain.CustomerMutationResult, error)
	upsertCustomerBatchAuthorized(context.Context, *domain.CustomerBatchUpsertRequest) (*domain.CustomerBatchUpsertResponse, error)
}

func NewLegacyContactAdapter(customers domain.CustomerService, maxBatchSize int) (*LegacyContactAdapter, error) {
	if customers == nil {
		return nil, errors.New("customer service is required")
	}
	if maxBatchSize <= 0 {
		return nil, errors.New("legacy contact batch limit must be positive")
	}
	return &LegacyContactAdapter{customers: customers, maxBatchSize: maxBatchSize}, nil
}

func (a *LegacyContactAdapter) BuildUpsertRequest(
	workspaceID string,
	operation string,
	mutation LegacyContactMutation,
) (*domain.UpsertCustomerRequest, error) {
	if a == nil || a.customers == nil {
		return nil, errors.New("legacy contact adapter is not configured")
	}
	operation = strings.TrimSpace(strings.ToLower(operation))
	switch operation {
	case LegacyContactOperationUpsert, LegacyContactOperationImport, LegacyContactOperationIngest:
	default:
		return nil, fmt.Errorf("unsupported legacy contact operation %q", operation)
	}
	if mutation.Contact == nil {
		return nil, errors.New("contact is required")
	}
	if err := mutation.Contact.Validate(); err != nil {
		return nil, err
	}

	input, err := legacyCustomerInput(mutation)
	if err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	payloadHash, err := input.CanonicalPayloadHash()
	if err != nil {
		return nil, err
	}
	request := &domain.UpsertCustomerRequest{
		WorkspaceID: workspaceID,
		IdempotencyKey: fmt.Sprintf("legacy-contact:%s:%s:%s",
			operation, mutation.Contact.Email, payloadHash),
		Customer: input,
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return request, nil
}

func (a *LegacyContactAdapter) Upsert(
	ctx context.Context,
	workspaceID string,
	operation string,
	mutation LegacyContactMutation,
) (*domain.CustomerMutationResult, error) {
	request, err := a.BuildUpsertRequest(workspaceID, operation, mutation)
	if err != nil {
		return nil, err
	}
	authorizedCtx := legacyCustomerAuthorityContext(ctx)
	if authority, ok := a.customers.(legacyCustomerAuthorityWriter); ok {
		return authority.upsertCustomerAuthorized(authorizedCtx, request)
	}
	return a.customers.UpsertCustomer(authorizedCtx, request)
}

func (a *LegacyContactAdapter) UpsertBatch(
	ctx context.Context,
	workspaceID string,
	operation string,
	mutations []LegacyContactMutation,
) (*domain.CustomerBatchUpsertResponse, error) {
	if len(mutations) == 0 {
		return nil, errors.New("legacy contact batch must contain at least one item")
	}
	if len(mutations) > a.maxBatchSize {
		return nil, fmt.Errorf("legacy contact batch cannot exceed %d items", a.maxBatchSize)
	}
	request := &domain.CustomerBatchUpsertRequest{
		WorkspaceID: workspaceID,
		Items:       make([]domain.CustomerBatchUpsertItem, len(mutations)),
	}
	for index := range mutations {
		item, err := a.BuildUpsertRequest(workspaceID, operation, mutations[index])
		if err != nil {
			return nil, fmt.Errorf("invalid legacy contact at index %d: %w", index, err)
		}
		request.Items[index] = domain.CustomerBatchUpsertItem{
			IdempotencyKey: item.IdempotencyKey,
			Customer:       item.Customer,
		}
	}
	if err := request.Validate(a.maxBatchSize); err != nil {
		return nil, err
	}
	authorizedCtx := legacyCustomerAuthorityContext(ctx)
	if authority, ok := a.customers.(legacyCustomerAuthorityWriter); ok {
		return authority.upsertCustomerBatchAuthorized(authorizedCtx, request)
	}
	return a.customers.UpsertCustomerBatch(authorizedCtx, request)
}

func legacyCustomerAuthorityContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, domain.SystemCallKey, true)
}

func legacyCustomerInput(mutation LegacyContactMutation) (domain.CustomerUpsertInput, error) {
	contact := mutation.Contact
	input := domain.CustomerUpsertInput{
		Identities: []domain.CustomerIdentityInput{{
			Type: domain.CustomerIdentityEmail, Value: contact.Email, Primary: true, Verified: false,
		}},
	}
	if contact.ExternalID != nil && !contact.ExternalID.IsNull {
		externalID := contact.ExternalID.String
		input.ExternalUserID = &externalID
	}

	profile := &domain.CustomerProfilePatch{}
	if contact.Language != nil && !contact.Language.IsNull {
		language := contact.Language.String
		profile.Language = &language
	}
	if contact.Timezone != nil && !contact.Timezone.IsNull {
		timezone := contact.Timezone.String
		profile.Timezone = &timezone
	}
	if mutation.Status != nil {
		status := *mutation.Status
		profile.Status = &status
	} else if contact.Profile != nil && contact.Profile.Status != nil {
		status := *contact.Profile.Status
		profile.Status = &status
	}

	attributes, err := legacyContactAttributes(contact)
	if err != nil {
		return domain.CustomerUpsertInput{}, err
	}
	if contact.Profile != nil {
		for key, value := range contact.Profile.Attributes {
			attributes[key] = value
		}
	}
	for key, value := range mutation.Attributes {
		attributes[key] = value
	}
	if len(attributes) > 0 {
		encoded, err := json.Marshal(attributes)
		if err != nil {
			return domain.CustomerUpsertInput{}, fmt.Errorf("encode legacy contact attributes: %w", err)
		}
		profile.Attributes = &domain.CustomerAttributesPatch{Merge: encoded}
	}
	if profile.Status != nil || profile.Language != nil || profile.Timezone != nil || profile.Attributes != nil {
		input.Profile = profile
	}

	if mutation.Tags != nil {
		tags := append([]string(nil), (*mutation.Tags)...)
		input.Tags = &tags
	} else if contact.Profile != nil && contact.Profile.Tags != nil {
		tags := append([]string(nil), contact.Profile.Tags...)
		input.Tags = &tags
	}
	if mutation.ListMemberships != nil {
		memberships := append([]domain.CustomerListMembershipInput(nil), (*mutation.ListMemberships)...)
		input.ListMemberships = &memberships
	}
	return input, nil
}

func legacyContactAttributes(contact *domain.Contact) (map[string]interface{}, error) {
	encoded, err := json.Marshal(contact)
	if err != nil {
		return nil, fmt.Errorf("encode legacy contact: %w", err)
	}
	attributes := make(map[string]interface{})
	if err := json.Unmarshal(encoded, &attributes); err != nil {
		return nil, fmt.Errorf("decode legacy contact attributes: %w", err)
	}
	for _, key := range []string{
		"email", "external_id", "timezone", "language", "profile", "created_at", "updated_at",
		"contact_lists", "contact_segments", "email_hmac",
	} {
		delete(attributes, key)
	}
	for key, value := range attributes {
		if value == nil {
			delete(attributes, key)
		}
	}
	return attributes, nil
}
