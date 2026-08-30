package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type legacyCustomerServiceStub struct {
	domain.CustomerService
	request      *domain.UpsertCustomerRequest
	batchRequest *domain.CustomerBatchUpsertRequest
	result       *domain.CustomerMutationResult
	batchResult  *domain.CustomerBatchUpsertResponse
	err          error
}

func (s *legacyCustomerServiceStub) UpsertCustomer(_ context.Context, request *domain.UpsertCustomerRequest) (*domain.CustomerMutationResult, error) {
	s.request = request
	return s.result, s.err
}

func (s *legacyCustomerServiceStub) UpsertCustomerBatch(_ context.Context, request *domain.CustomerBatchUpsertRequest) (*domain.CustomerBatchUpsertResponse, error) {
	s.batchRequest = request
	return s.batchResult, s.err
}

func TestLegacyContactAdapterBuildsPrimaryEmailCustomerCommand(t *testing.T) {
	customerService := &legacyCustomerServiceStub{result: &domain.CustomerMutationResult{CustomerID: "customer-1", Action: "created"}}
	adapter, err := NewLegacyContactAdapter(customerService, 10_000)
	require.NoError(t, err)

	externalID := &domain.NullableString{String: "crm-42"}
	firstName := &domain.NullableString{String: "Ada"}
	language := &domain.NullableString{String: "zh-CN"}
	timezone := &domain.NullableString{String: "Asia/Shanghai"}
	contact := &domain.Contact{
		Email: " ADA@Example.com ", ExternalID: externalID, FirstName: firstName,
		Language: language, Timezone: timezone,
	}

	_, err = adapter.Upsert(context.Background(), "workspace1", LegacyContactOperationUpsert, LegacyContactMutation{Contact: contact})
	require.NoError(t, err)
	require.NotNil(t, customerService.request)
	assert.Equal(t, "workspace1", customerService.request.WorkspaceID)
	assert.Equal(t, "ada@example.com", customerService.request.Customer.Identities[0].Value)
	assert.True(t, customerService.request.Customer.Identities[0].Primary)
	assert.Equal(t, domain.CustomerIdentityEmail, customerService.request.Customer.Identities[0].Type)
	require.NotNil(t, customerService.request.Customer.ExternalUserID)
	assert.Equal(t, "crm-42", *customerService.request.Customer.ExternalUserID)
	require.NotNil(t, customerService.request.Customer.Profile)
	assert.Equal(t, "zh-CN", *customerService.request.Customer.Profile.Language)
	assert.Equal(t, "Asia/Shanghai", *customerService.request.Customer.Profile.Timezone)

	var attributes map[string]interface{}
	require.NoError(t, json.Unmarshal(customerService.request.Customer.Profile.Attributes.Merge, &attributes))
	assert.Equal(t, "Ada", attributes["first_name"])
	assert.Regexp(t, `^legacy-contact:upsert:ada@example\.com:[0-9a-f]{64}$`, customerService.request.IdempotencyKey)
}

func TestLegacyContactAdapterIdempotencyChangesOnlyWhenCanonicalPayloadChanges(t *testing.T) {
	adapter, err := NewLegacyContactAdapter(&legacyCustomerServiceStub{}, 10_000)
	require.NoError(t, err)

	first := &domain.Contact{Email: "USER@example.com", FirstName: &domain.NullableString{String: "Ada"}}
	equivalent := &domain.Contact{Email: " user@example.com ", FirstName: &domain.NullableString{String: "Ada"}}
	changed := &domain.Contact{Email: "user@example.com", FirstName: &domain.NullableString{String: "Grace"}}

	request1, err := adapter.BuildUpsertRequest("workspace1", LegacyContactOperationUpsert, LegacyContactMutation{Contact: first})
	require.NoError(t, err)
	request2, err := adapter.BuildUpsertRequest("workspace1", LegacyContactOperationUpsert, LegacyContactMutation{Contact: equivalent})
	require.NoError(t, err)
	request3, err := adapter.BuildUpsertRequest("workspace1", LegacyContactOperationUpsert, LegacyContactMutation{Contact: changed})
	require.NoError(t, err)

	assert.Equal(t, request1.IdempotencyKey, request2.IdempotencyKey)
	assert.NotEqual(t, request1.IdempotencyKey, request3.IdempotencyKey)
}

func TestLegacyContactAdapterMapsIngestProfileTagsAndLists(t *testing.T) {
	adapter, err := NewLegacyContactAdapter(&legacyCustomerServiceStub{}, 10_000)
	require.NoError(t, err)
	status := "active"
	tags := []string{"paid", "vip"}
	memberships := []domain.CustomerListMembershipInput{{ListID: "news", Status: "active"}}

	request, err := adapter.BuildUpsertRequest("workspace1", LegacyContactOperationIngest, LegacyContactMutation{
		Contact:         &domain.Contact{Email: "USER@example.com"},
		Status:          &status,
		Attributes:      map[string]interface{}{"plan": "pro"},
		Tags:            &tags,
		ListMemberships: &memberships,
	})
	require.NoError(t, err)
	require.NotNil(t, request.Customer.Profile)
	assert.Equal(t, "active", *request.Customer.Profile.Status)
	var attributes map[string]interface{}
	require.NoError(t, json.Unmarshal(request.Customer.Profile.Attributes.Merge, &attributes))
	assert.Equal(t, "pro", attributes["plan"])
	require.NotNil(t, request.Customer.Tags)
	assert.Equal(t, []string{"paid", "vip"}, *request.Customer.Tags)
	require.NotNil(t, request.Customer.ListMemberships)
	assert.Equal(t, memberships, *request.Customer.ListMemberships)
}

func TestLegacyContactAdapterBatchPreservesInputOrderAndConfiguredLimit(t *testing.T) {
	customerService := &legacyCustomerServiceStub{batchResult: &domain.CustomerBatchUpsertResponse{Results: []domain.CustomerBatchItemResult{
		{Index: 0, Status: "accepted"}, {Index: 1, Status: "error"},
	}}}
	adapter, err := NewLegacyContactAdapter(customerService, 2)
	require.NoError(t, err)
	mutations := []LegacyContactMutation{
		{Contact: &domain.Contact{Email: "first@example.com"}},
		{Contact: &domain.Contact{Email: "second@example.com"}},
	}

	response, err := adapter.UpsertBatch(context.Background(), "workspace1", LegacyContactOperationImport, mutations)
	require.NoError(t, err)
	require.Len(t, customerService.batchRequest.Items, 2)
	assert.Equal(t, "first@example.com", customerService.batchRequest.Items[0].Customer.Identities[0].Value)
	assert.Equal(t, "second@example.com", customerService.batchRequest.Items[1].Customer.Identities[0].Value)
	assert.Equal(t, []int{0, 1}, []int{response.Results[0].Index, response.Results[1].Index})

	_, err = adapter.UpsertBatch(context.Background(), "workspace1", LegacyContactOperationImport, append(mutations,
		LegacyContactMutation{Contact: &domain.Contact{Email: "third@example.com"}},
	))
	assert.ErrorContains(t, err, "cannot exceed 2")
}
