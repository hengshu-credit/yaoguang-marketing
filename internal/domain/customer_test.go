package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCustomerNumberUsesWorkspaceSequenceShanghaiTimeAndFullUUID(t *testing.T) {
	id := uuid.MustParse("A9B4C7D2-1F6E-4D01-A932-5FC80B7312AE")
	instant := time.Date(2026, time.August, 29, 7, 30, 45, 987654321, time.UTC)

	got, err := GenerateCustomerNumber(27, instant, id)

	require.NoError(t, err)
	assert.Equal(t, "U00272026082915304508a9b4c7d21f6e4d01a9325fc80b7312ae", got)
	assert.Len(t, got, 53)
}

func TestGenerateCustomerNumberAcceptsSequenceBoundaries(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	instant := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("UTC-5", -5*60*60))

	tests := []struct {
		name     string
		sequence uint16
		want     string
	}{
		{
			name:     "first workspace",
			sequence: 1,
			want:     "U0001202601021604050800000000000000000000000000000001",
		},
		{
			name:     "last workspace",
			sequence: 9999,
			want:     "U9999202601021604050800000000000000000000000000000001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateCustomerNumber(tt.sequence, instant, id)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateCustomerNumberRejectsUnallocatedWorkspaceSequence(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	got, err := GenerateCustomerNumber(0, time.Now(), id)

	assert.Empty(t, got)
	require.Error(t, err)
	assert.ErrorContains(t, err, "workspace sequence")
}

func TestGenerateCustomerNumberRejectsNilCustomerID(t *testing.T) {
	got, err := GenerateCustomerNumber(1, time.Now(), uuid.Nil)

	assert.Empty(t, got)
	require.Error(t, err)
	assert.ErrorContains(t, err, "customer ID")
}

func TestCustomerLocatorValidateRequiresExactlyOneLookupForm(t *testing.T) {
	validID := "a9b4c7d2-1f6e-4d01-a932-5fc80b7312ae"
	validNumber := "U00272026082915304508a9b4c7d21f6e4d01a9325fc80b7312ae"

	tests := []struct {
		name    string
		locator CustomerLocator
		wantErr string
	}{
		{name: "customer id", locator: CustomerLocator{CustomerID: validID}},
		{name: "customer number", locator: CustomerLocator{CustomerNo: validNumber}},
		{name: "external user id", locator: CustomerLocator{ExternalUserID: " core-user-42 "}},
		{name: "identity", locator: CustomerLocator{Identity: &CustomerIdentityLocator{Type: CustomerIdentityEmail, Value: " Alice@Example.COM "}}},
		{name: "missing", locator: CustomerLocator{}, wantErr: "exactly one"},
		{
			name:    "ambiguous",
			locator: CustomerLocator{CustomerID: validID, ExternalUserID: "core-user-42"},
			wantErr: "exactly one",
		},
		{name: "malformed uuid", locator: CustomerLocator{CustomerID: "not-a-uuid"}, wantErr: "customer_id"},
		{name: "malformed customer number", locator: CustomerLocator{CustomerNo: "U27"}, wantErr: "customer_no"},
		{name: "blank external id", locator: CustomerLocator{ExternalUserID: "   "}, wantErr: "external_user_id"},
		{name: "external id too long", locator: CustomerLocator{ExternalUserID: strings.Repeat("x", 256)}, wantErr: "external_user_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locator := tt.locator
			err := locator.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if locator.ExternalUserID != "" {
				assert.Equal(t, "core-user-42", locator.ExternalUserID)
			}
			if locator.Identity != nil {
				assert.Equal(t, "alice@example.com", locator.Identity.Value)
			}
		})
	}
}

func TestNormalizeCustomerIdentityUsesTypeSpecificCanonicalValues(t *testing.T) {
	tests := []struct {
		name  string
		input CustomerIdentityInput
		want  NormalizedCustomerIdentity
	}{
		{
			name:  "email is trimmed and lowercased",
			input: CustomerIdentityInput{Type: CustomerIdentityEmail, Value: " Alice+VIP@Example.COM "},
			want:  NormalizedCustomerIdentity{Type: CustomerIdentityEmail, Value: "alice+vip@example.com", DisplayHint: "a***@example.com"},
		},
		{
			name:  "phone formatting is removed",
			input: CustomerIdentityInput{Type: CustomerIdentityPhone, Value: "+86 (138) 0013-8000"},
			want:  NormalizedCustomerIdentity{Type: CustomerIdentityPhone, Value: "+8613800138000", DisplayHint: "+86*******8000"},
		},
		{
			name:  "whatsapp uses E164",
			input: CustomerIdentityInput{Type: CustomerIdentityWhatsApp, Value: "0086 138 0013 8000"},
			want:  NormalizedCustomerIdentity{Type: CustomerIdentityWhatsApp, Value: "+8613800138000", DisplayHint: "+86*******8000"},
		},
		{
			name:  "provider identifier preserves case",
			input: CustomerIdentityInput{Type: CustomerIdentityDeviceID, Value: " Device-AbC-42 "},
			want:  NormalizedCustomerIdentity{Type: CustomerIdentityDeviceID, Value: "Device-AbC-42", DisplayHint: "De*********42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeCustomerIdentity(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeCustomerIdentityRejectsUnsupportedOrMalformedValues(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomerIdentityInput
		wantErr string
	}{
		{name: "unsupported type", input: CustomerIdentityInput{Type: "social", Value: "abc"}, wantErr: "identity type"},
		{name: "invalid email", input: CustomerIdentityInput{Type: CustomerIdentityEmail, Value: "not-an-email"}, wantErr: "email"},
		{name: "domestic phone has no country code", input: CustomerIdentityInput{Type: CustomerIdentityPhone, Value: "13800138000"}, wantErr: "E.164"},
		{name: "blank provider id", input: CustomerIdentityInput{Type: CustomerIdentityAnonymousID, Value: "   "}, wantErr: "identity value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeCustomerIdentity(tt.input)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCustomerIdentityFingerprintScopesNormalizedValueByType(t *testing.T) {
	email, err := NormalizeCustomerIdentity(CustomerIdentityInput{Type: CustomerIdentityEmail, Value: " Alice@Example.COM "})
	require.NoError(t, err)
	custom, err := NormalizeCustomerIdentity(CustomerIdentityInput{Type: CustomerIdentityCustom, Value: "alice@example.com"})
	require.NoError(t, err)

	emailFingerprint, err := CustomerIdentityFingerprint("workspace-secret", email)
	require.NoError(t, err)
	customFingerprint, err := CustomerIdentityFingerprint("workspace-secret", custom)
	require.NoError(t, err)

	assert.Len(t, emailFingerprint, 64)
	assert.NotEqual(t, emailFingerprint, customFingerprint)
	assert.NotContains(t, emailFingerprint, email.Value)

	_, err = CustomerIdentityFingerprint("", email)
	assert.ErrorContains(t, err, "secret")
}

func TestApplyCustomerAttributesPatchSetsMergesAndUnsetsWithoutDeletingJSONNull(t *testing.T) {
	current := map[string]interface{}{
		"discarded": true,
	}
	patch := &CustomerAttributesPatch{
		Set:   json.RawMessage(`{"account":{"tier":"gold","note":null},"keep":1}`),
		Merge: json.RawMessage(`{"account":{"score":8},"added":true}`),
		Unset: []string{"account.tier"},
	}

	got, err := ApplyCustomerAttributesPatch(current, patch)

	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"account": map[string]interface{}{"note": nil, "score": float64(8)},
		"keep":    float64(1),
		"added":   true,
	}, got)
	assert.Equal(t, map[string]interface{}{"discarded": true}, current, "the stored input must not be mutated")
}

func TestApplyCustomerAttributesPatchRecursivelyMergesExistingObject(t *testing.T) {
	current := map[string]interface{}{
		"account": map[string]interface{}{"tier": "silver", "score": float64(2)},
		"keep":    "yes",
	}
	patch := &CustomerAttributesPatch{
		Merge: json.RawMessage(`{"account":{"score":5,"note":null}}`),
		Unset: []string{"keep"},
	}

	got, err := ApplyCustomerAttributesPatch(current, patch)

	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"account": map[string]interface{}{"tier": "silver", "score": float64(5), "note": nil},
	}, got)
}

func TestApplyCustomerAttributesPatchRejectsAmbiguousOrMalformedOperations(t *testing.T) {
	tests := []struct {
		name    string
		patch   *CustomerAttributesPatch
		wantErr string
	}{
		{name: "set null is not deletion", patch: &CustomerAttributesPatch{Set: json.RawMessage(`null`)}, wantErr: "set must be a JSON object"},
		{name: "merge array", patch: &CustomerAttributesPatch{Merge: json.RawMessage(`[]`)}, wantErr: "merge must be a JSON object"},
		{name: "empty unset segment", patch: &CustomerAttributesPatch{Unset: []string{"account..tier"}}, wantErr: "unset path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ApplyCustomerAttributesPatch(map[string]interface{}{}, tt.patch)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCustomerUpsertInputValidateAllowsExternalIDWithoutEmailAndCanonicalizesChildren(t *testing.T) {
	externalID := " core-user-42 "
	tags := []string{" vip ", "active", "vip"}
	input := CustomerUpsertInput{
		ExternalUserID: &externalID,
		Profile: &CustomerProfilePatch{
			Status:     stringPointer(" active "),
			Language:   stringPointer(" zh-CN "),
			Timezone:   stringPointer(" Asia/Shanghai "),
			Attributes: &CustomerAttributesPatch{Merge: json.RawMessage(`{"risk":{"level":"low"}}`)},
		},
		Identities: []CustomerIdentityInput{
			{Type: CustomerIdentityEmail, Value: " Alice@Example.COM ", Primary: true},
			{Type: CustomerIdentityDeviceID, Value: " Device-AbC-42 "},
		},
		Tags: &tags,
		ListMemberships: []CustomerListMembershipInput{
			{ListID: "list123", Status: " active "},
		},
	}

	err := input.Validate()

	require.NoError(t, err)
	assert.Equal(t, "core-user-42", *input.ExternalUserID)
	assert.Equal(t, "active", *input.Profile.Status)
	assert.Equal(t, "zh-CN", *input.Profile.Language)
	assert.Equal(t, "Asia/Shanghai", *input.Profile.Timezone)
	assert.Equal(t, "alice@example.com", input.Identities[0].Value)
	assert.Equal(t, "Device-AbC-42", input.Identities[1].Value)
	assert.Equal(t, []string{"active", "vip"}, *input.Tags)
	assert.Equal(t, "active", input.ListMemberships[0].Status)
}

func TestCustomerUpsertInputValidateRequiresResolvableIdentityAndRejectsNormalizedDuplicates(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomerUpsertInput
		wantErr string
	}{
		{name: "no locator", input: CustomerUpsertInput{}, wantErr: "locator, external_user_id, or identity"},
		{
			name: "duplicate normalized identity",
			input: CustomerUpsertInput{Identities: []CustomerIdentityInput{
				{Type: CustomerIdentityEmail, Value: "alice@example.com"},
				{Type: CustomerIdentityEmail, Value: " Alice@Example.com "},
			}},
			wantErr: "duplicate identity",
		},
		{
			name: "two primary emails",
			input: CustomerUpsertInput{Identities: []CustomerIdentityInput{
				{Type: CustomerIdentityEmail, Value: "alice@example.com", Primary: true},
				{Type: CustomerIdentityEmail, Value: "other@example.com", Primary: true},
			}},
			wantErr: "primary email",
		},
		{
			name:    "invalid tag",
			input:   CustomerUpsertInput{ExternalUserID: stringPointer("user-1"), Tags: stringSlicePointer([]string{""})},
			wantErr: "tag",
		},
		{
			name: "invalid list status",
			input: CustomerUpsertInput{
				ExternalUserID:  stringPointer("user-1"),
				ListMemberships: []CustomerListMembershipInput{{ListID: "list123", Status: "unknown"}},
			},
			wantErr: "list membership status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCustomerUpsertInputCanonicalPayloadHashUsesNormalizedBusinessPayload(t *testing.T) {
	first := CustomerUpsertInput{
		ExternalUserID: stringPointer(" core-user-42 "),
		Identities:     []CustomerIdentityInput{{Type: CustomerIdentityEmail, Value: "Alice@Example.COM"}},
		Profile: &CustomerProfilePatch{
			Attributes: &CustomerAttributesPatch{Merge: json.RawMessage(`{"tier":"gold"}`)},
		},
	}
	second := CustomerUpsertInput{
		ExternalUserID: stringPointer("core-user-42"),
		Identities:     []CustomerIdentityInput{{Type: CustomerIdentityEmail, Value: " alice@example.com "}},
		Profile: &CustomerProfilePatch{
			Attributes: &CustomerAttributesPatch{Merge: json.RawMessage(`{ "tier" : "gold" }`)},
		},
	}
	require.NoError(t, first.Validate())
	require.NoError(t, second.Validate())

	firstHash, err := first.CanonicalPayloadHash()
	require.NoError(t, err)
	secondHash, err := second.CanonicalPayloadHash()
	require.NoError(t, err)

	assert.Regexp(t, `^[0-9a-f]{64}$`, firstHash)
	assert.Equal(t, firstHash, secondHash)

	*second.ExternalUserID = "core-user-43"
	differentHash, err := second.CanonicalPayloadHash()
	require.NoError(t, err)
	assert.NotEqual(t, firstHash, differentHash)
}

func TestUpsertCustomerRequestValidateRequiresWorkspaceAndIdempotencyKey(t *testing.T) {
	tests := []struct {
		name    string
		request UpsertCustomerRequest
		wantErr string
	}{
		{
			name: "valid",
			request: UpsertCustomerRequest{
				WorkspaceID:    "workspace123",
				IdempotencyKey: "crm-sync-42",
				Customer:       CustomerUpsertInput{ExternalUserID: stringPointer("core-user-42")},
			},
		},
		{
			name: "missing workspace",
			request: UpsertCustomerRequest{
				IdempotencyKey: "crm-sync-42",
				Customer:       CustomerUpsertInput{ExternalUserID: stringPointer("core-user-42")},
			},
			wantErr: "workspace_id",
		},
		{
			name: "missing idempotency key",
			request: UpsertCustomerRequest{
				WorkspaceID: "workspace123",
				Customer:    CustomerUpsertInput{ExternalUserID: stringPointer("core-user-42")},
			},
			wantErr: "idempotency_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func stringPointer(value string) *string { return &value }

func stringSlicePointer(value []string) *[]string { return &value }
