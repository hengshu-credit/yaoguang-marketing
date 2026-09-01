package domain

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

const (
	MinWorkspaceSequence uint16 = 1
	MaxWorkspaceSequence uint16 = 9999
)

var yaoguangCustomerNumberLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

var (
	customerNumberPattern = regexp.MustCompile(`^(?:U[0-9a-z]{3}[0-9]{14}08[0-9a-z]{6}|U[0-9]{4}[0-9]{14}08[0-9a-f]{32})$`)
	customerE164Pattern   = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
)

const customerNumberRandomSpace uint64 = 36 * 36 * 36 * 36 * 36 * 36

type CustomerIdentityType string

const (
	CustomerIdentityEmail       CustomerIdentityType = "email"
	CustomerIdentityPhone       CustomerIdentityType = "phone"
	CustomerIdentityAnonymousID CustomerIdentityType = "anonymous_id"
	CustomerIdentityDeviceID    CustomerIdentityType = "device_id"
	CustomerIdentityWhatsApp    CustomerIdentityType = "whatsapp"
	CustomerIdentityTelegram    CustomerIdentityType = "telegram"
	CustomerIdentityCustom      CustomerIdentityType = "custom"
)

var supportedCustomerIdentityTypes = map[CustomerIdentityType]struct{}{
	CustomerIdentityEmail:       {},
	CustomerIdentityPhone:       {},
	CustomerIdentityAnonymousID: {},
	CustomerIdentityDeviceID:    {},
	CustomerIdentityWhatsApp:    {},
	CustomerIdentityTelegram:    {},
	CustomerIdentityCustom:      {},
}

// CustomerIdentityInput carries a plaintext identity only across the inbound
// API boundary. Repositories encrypt Value before persistence and responses use
// DisplayHint instead of returning Value.
type CustomerIdentityInput struct {
	Type     CustomerIdentityType   `json:"type"`
	Value    string                 `json:"value"`
	Verified bool                   `json:"verified,omitempty"`
	Primary  bool                   `json:"primary,omitempty"`
	Enabled  *bool                  `json:"enabled,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type NormalizedCustomerIdentity struct {
	Type        CustomerIdentityType `json:"type"`
	Value       string               `json:"-"`
	DisplayHint string               `json:"display_hint"`
}

type CustomerIdentityLocator struct {
	Type  CustomerIdentityType `json:"type"`
	Value string               `json:"value"`
}

type CustomerLocator struct {
	CustomerID     string                   `json:"customer_id,omitempty"`
	CustomerNo     string                   `json:"customer_no,omitempty"`
	ExternalUserID string                   `json:"external_user_id,omitempty"`
	Identity       *CustomerIdentityLocator `json:"identity,omitempty"`
}

// Validate requires one unambiguous lookup form and canonicalizes the external
// or identity value in place so repositories never implement their own rules.
func (l *CustomerLocator) Validate() error {
	if l == nil {
		return fmt.Errorf("customer locator is required")
	}

	forms := 0
	if l.CustomerID != "" {
		forms++
	}
	if l.CustomerNo != "" {
		forms++
	}
	if l.ExternalUserID != "" {
		forms++
	}
	if l.Identity != nil {
		forms++
	}
	if forms != 1 {
		return fmt.Errorf("customer locator must contain exactly one lookup form")
	}

	if l.CustomerID != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(l.CustomerID))
		if err != nil || parsed == uuid.Nil {
			return fmt.Errorf("customer_id must be a non-nil UUID")
		}
		l.CustomerID = parsed.String()
		return nil
	}
	if l.CustomerNo != "" {
		l.CustomerNo = strings.TrimSpace(l.CustomerNo)
		if !customerNumberPattern.MatchString(l.CustomerNo) {
			return fmt.Errorf("customer_no must match the Yaoguang customer number format")
		}
		return nil
	}
	if l.ExternalUserID != "" {
		l.ExternalUserID = strings.TrimSpace(l.ExternalUserID)
		if l.ExternalUserID == "" || utf8.RuneCountInString(l.ExternalUserID) > 255 {
			return fmt.Errorf("external_user_id must contain 1 to 255 characters")
		}
		return nil
	}

	normalized, err := NormalizeCustomerIdentity(CustomerIdentityInput{
		Type:  l.Identity.Type,
		Value: l.Identity.Value,
	})
	if err != nil {
		return err
	}
	l.Identity.Type = normalized.Type
	l.Identity.Value = normalized.Value
	return nil
}

func NormalizeCustomerIdentity(input CustomerIdentityInput) (NormalizedCustomerIdentity, error) {
	identityType := CustomerIdentityType(strings.TrimSpace(strings.ToLower(string(input.Type))))
	if _, ok := supportedCustomerIdentityTypes[identityType]; !ok {
		return NormalizedCustomerIdentity{}, fmt.Errorf("unsupported identity type %q", input.Type)
	}

	value := trimUnicodeSpace(input.Value)
	if value == "" || utf8.RuneCountInString(value) > 1024 {
		return NormalizedCustomerIdentity{}, fmt.Errorf("identity value must contain 1 to 1024 characters")
	}

	switch identityType {
	case CustomerIdentityEmail:
		value = NormalizeEmail(value)
		if !govalidator.IsEmail(value) {
			return NormalizedCustomerIdentity{}, fmt.Errorf("identity email is invalid")
		}
	case CustomerIdentityPhone, CustomerIdentityWhatsApp:
		var err error
		value, err = normalizeCustomerE164(value)
		if err != nil {
			return NormalizedCustomerIdentity{}, err
		}
	}

	return NormalizedCustomerIdentity{
		Type:        identityType,
		Value:       value,
		DisplayHint: maskCustomerIdentity(identityType, value),
	}, nil
}

// CustomerIdentityFingerprint produces a deterministic lookup key without
// exposing the normalized identity value in indexes or query logs.
func CustomerIdentityFingerprint(secretKey string, identity NormalizedCustomerIdentity) (string, error) {
	if strings.TrimSpace(secretKey) == "" {
		return "", fmt.Errorf("customer identity fingerprint secret must not be empty")
	}
	if identity.Type == "" || identity.Value == "" {
		return "", fmt.Errorf("normalized customer identity must include type and value")
	}
	return pkgcrypto.ComputeHMAC256([]byte(string(identity.Type)+"\x00"+identity.Value), secretKey), nil
}

// CustomerIdentityFingerprintForWorkspace derives a workspace-specific HMAC
// key first, preventing the same identity from being correlated across
// otherwise isolated Workspace databases.
func CustomerIdentityFingerprintForWorkspace(secretKey, workspaceID string, identity NormalizedCustomerIdentity) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", fmt.Errorf("workspace ID is required for customer identity fingerprint")
	}
	if strings.TrimSpace(secretKey) == "" {
		return "", fmt.Errorf("customer identity fingerprint secret must not be empty")
	}
	workspaceKey := pkgcrypto.ComputeHMAC256([]byte("customer_identity_workspace\x00"+workspaceID), secretKey)
	return CustomerIdentityFingerprint(workspaceKey, identity)
}

func normalizeCustomerE164(value string) (string, error) {
	value = trimUnicodeSpace(value)
	if strings.HasPrefix(value, "00") {
		value = "+" + value[2:]
	}
	var normalized strings.Builder
	for index, r := range value {
		switch {
		case r >= '0' && r <= '9':
			normalized.WriteRune(r)
		case r == '+' && index == 0:
			normalized.WriteRune(r)
		case r == ' ' || r == '\t' || r == '-' || r == '(' || r == ')' || r == '.':
			continue
		default:
			return "", fmt.Errorf("phone identity must be in E.164 format")
		}
	}
	result := normalized.String()
	if !customerE164Pattern.MatchString(result) {
		return "", fmt.Errorf("phone identity must be in E.164 format")
	}
	return result, nil
}

func maskCustomerIdentity(identityType CustomerIdentityType, value string) string {
	switch identityType {
	case CustomerIdentityEmail:
		parts := strings.SplitN(value, "@", 2)
		if len(parts) == 2 && parts[0] != "" {
			return string([]rune(parts[0])[0]) + "***@" + parts[1]
		}
	case CustomerIdentityPhone, CustomerIdentityWhatsApp:
		runes := []rune(value)
		if len(runes) > 7 {
			return string(runes[:3]) + strings.Repeat("*", len(runes)-7) + string(runes[len(runes)-4:])
		}
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:2]) + strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-2:])
}

type CustomerAttributesPatch struct {
	Set   json.RawMessage `json:"set,omitempty"`
	Merge json.RawMessage `json:"merge,omitempty"`
	Unset []string        `json:"unset,omitempty"`
}

type CustomerProfile struct {
	CustomerID string                 `json:"customer_id"`
	Status     *string                `json:"status,omitempty"`
	Language   *string                `json:"language,omitempty"`
	Timezone   *string                `json:"timezone,omitempty"`
	Attributes map[string]interface{} `json:"attributes"`
	Version    int64                  `json:"version"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type CustomerProfilePatch struct {
	Status     *string                  `json:"status,omitempty"`
	Language   *string                  `json:"language,omitempty"`
	Timezone   *string                  `json:"timezone,omitempty"`
	Attributes *CustomerAttributesPatch `json:"attributes,omitempty"`
}

func (p *CustomerProfilePatch) Validate() error {
	if p == nil {
		return nil
	}
	if err := normalizeOptionalCustomerProfileString("status", p.Status, 64); err != nil {
		return err
	}
	if err := normalizeOptionalCustomerProfileString("language", p.Language, 50); err != nil {
		return err
	}
	if p.Language != nil && !endpointLocalePattern.MatchString(*p.Language) {
		return fmt.Errorf("profile language is invalid")
	}
	if err := normalizeOptionalCustomerProfileString("timezone", p.Timezone, 50); err != nil {
		return err
	}
	if p.Timezone != nil && !IsValidTimezone(*p.Timezone) {
		return fmt.Errorf("profile timezone is invalid")
	}
	if p.Attributes != nil {
		if _, err := ApplyCustomerAttributesPatch(map[string]interface{}{}, p.Attributes); err != nil {
			return fmt.Errorf("invalid profile attributes: %w", err)
		}
		var err error
		p.Attributes.Set, err = canonicalCustomerAttributeJSON("set", p.Attributes.Set)
		if err != nil {
			return err
		}
		p.Attributes.Merge, err = canonicalCustomerAttributeJSON("merge", p.Attributes.Merge)
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizeOptionalCustomerProfileString(field string, value *string, maxLength int) error {
	if value == nil {
		return nil
	}
	*value = trimUnicodeSpace(*value)
	if *value == "" || utf8.RuneCountInString(*value) > maxLength {
		return fmt.Errorf("profile %s must contain 1 to %d characters", field, maxLength)
	}
	return nil
}

func canonicalCustomerAttributeJSON(operation string, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	object, err := decodeCustomerAttributeObject(operation, raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode canonical %s attributes: %w", operation, err)
	}
	return encoded, nil
}

type CustomerListMembershipInput struct {
	ListID string `json:"list_id"`
	Status string `json:"status,omitempty"`
}

const (
	MaxCustomerListMembershipBatchCustomers = 50
	MaxCustomerListMembershipBatchLists     = 50
)

type CustomerListMembershipAction string

const (
	CustomerListMembershipActionAdd       CustomerListMembershipAction = "add"
	CustomerListMembershipActionRemove    CustomerListMembershipAction = "remove"
	CustomerListMembershipActionSetStatus CustomerListMembershipAction = "set_status"
)

type CustomerListMembershipStatus = ContactListStatus

const (
	CustomerListMembershipStatusActive       = ContactListStatusActive
	CustomerListMembershipStatusPending      = ContactListStatusPending
	CustomerListMembershipStatusUnsubscribed = ContactListStatusUnsubscribed
	CustomerListMembershipStatusBounced      = ContactListStatusBounced
	CustomerListMembershipStatusComplained   = ContactListStatusComplained
)

type CustomerListMembershipUpdateRequest struct {
	WorkspaceID string                       `json:"workspace_id"`
	CustomerIDs []string                     `json:"customer_ids"`
	ListIDs     []string                     `json:"list_ids"`
	Action      CustomerListMembershipAction `json:"action"`
	Status      CustomerListMembershipStatus `json:"status,omitempty"`
}

type CustomerListMembershipUpdateResult struct {
	Customers int `json:"customers"`
	Lists     int `json:"lists"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
}

func (request *CustomerListMembershipUpdateRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || utf8.RuneCountInString(request.WorkspaceID) > 32 || !govalidator.IsAlphanumeric(request.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric and contain 1 to 32 characters")
	}
	if len(request.CustomerIDs) == 0 {
		return fmt.Errorf("customer_ids must contain at least one customer")
	}
	if len(request.CustomerIDs) > MaxCustomerListMembershipBatchCustomers {
		return fmt.Errorf("customer_ids cannot contain more than %d customers", MaxCustomerListMembershipBatchCustomers)
	}
	seenCustomers := make(map[string]struct{}, len(request.CustomerIDs))
	for index, rawCustomerID := range request.CustomerIDs {
		parsed, err := uuid.Parse(strings.TrimSpace(rawCustomerID))
		if err != nil || parsed == uuid.Nil {
			return fmt.Errorf("customer_ids[%d] must be a non-nil UUID", index)
		}
		customerID := parsed.String()
		if _, exists := seenCustomers[customerID]; exists {
			return fmt.Errorf("duplicate customer_id %s", customerID)
		}
		seenCustomers[customerID] = struct{}{}
		request.CustomerIDs[index] = customerID
	}
	if len(request.ListIDs) == 0 {
		return fmt.Errorf("list_ids must contain at least one list")
	}
	if len(request.ListIDs) > MaxCustomerListMembershipBatchLists {
		return fmt.Errorf("list_ids cannot contain more than %d lists", MaxCustomerListMembershipBatchLists)
	}
	seenLists := make(map[string]struct{}, len(request.ListIDs))
	for index, rawListID := range request.ListIDs {
		listID := trimUnicodeSpace(rawListID)
		if listID == "" || utf8.RuneCountInString(listID) > 32 || !govalidator.IsAlphanumeric(listID) {
			return fmt.Errorf("list_ids[%d] must be alphanumeric and contain 1 to 32 characters", index)
		}
		if _, exists := seenLists[listID]; exists {
			return fmt.Errorf("duplicate list_id %s", listID)
		}
		seenLists[listID] = struct{}{}
		request.ListIDs[index] = listID
	}
	request.Action = CustomerListMembershipAction(strings.ToLower(strings.TrimSpace(string(request.Action))))
	request.Status = CustomerListMembershipStatus(strings.ToLower(strings.TrimSpace(string(request.Status))))
	switch request.Action {
	case CustomerListMembershipActionAdd:
		if request.Status == "" {
			request.Status = CustomerListMembershipStatusActive
		}
	case CustomerListMembershipActionSetStatus:
		if request.Status == "" {
			return fmt.Errorf("status is required when action is set_status")
		}
	case CustomerListMembershipActionRemove:
		if request.Status != "" {
			return fmt.Errorf("status is not allowed when action is remove")
		}
		return nil
	default:
		return fmt.Errorf("action must be add, remove, or set_status")
	}
	switch request.Status {
	case CustomerListMembershipStatusActive, CustomerListMembershipStatusPending,
		CustomerListMembershipStatusUnsubscribed, CustomerListMembershipStatusBounced,
		CustomerListMembershipStatusComplained:
		return nil
	default:
		return fmt.Errorf("status must be active, pending, unsubscribed, bounced, or complained")
	}
}

type CustomerUpsertInput struct {
	Locator            *CustomerLocator               `json:"locator,omitempty"`
	ExternalUserID     *string                        `json:"external_user_id,omitempty"`
	Profile            *CustomerProfilePatch          `json:"profile,omitempty"`
	Identities         []CustomerIdentityInput        `json:"identities,omitempty"`
	Tags               *[]string                      `json:"tags,omitempty"`
	ListMemberships    *[]CustomerListMembershipInput `json:"list_memberships,omitempty"`
	ListMembershipsAdd *[]CustomerListMembershipInput `json:"list_memberships_add,omitempty"`
}

func (input *CustomerUpsertInput) Validate() error {
	if input == nil {
		return fmt.Errorf("customer is required")
	}
	if input.Locator != nil {
		if err := input.Locator.Validate(); err != nil {
			return err
		}
	}
	if input.ExternalUserID != nil {
		*input.ExternalUserID = trimUnicodeSpace(*input.ExternalUserID)
		if *input.ExternalUserID == "" || utf8.RuneCountInString(*input.ExternalUserID) > 255 {
			return fmt.Errorf("external_user_id must contain 1 to 255 characters")
		}
	}
	if input.Locator == nil && input.ExternalUserID == nil && len(input.Identities) == 0 {
		return fmt.Errorf("customer requires a locator, external_user_id, or identity")
	}

	identityKeys := make(map[string]struct{}, len(input.Identities))
	primaryByType := make(map[CustomerIdentityType]int)
	for index := range input.Identities {
		normalized, err := NormalizeCustomerIdentity(input.Identities[index])
		if err != nil {
			return fmt.Errorf("identity %d: %w", index, err)
		}
		input.Identities[index].Type = normalized.Type
		input.Identities[index].Value = normalized.Value
		key := string(normalized.Type) + "\x00" + normalized.Value
		if _, exists := identityKeys[key]; exists {
			return fmt.Errorf("duplicate identity %s", normalized.Type)
		}
		identityKeys[key] = struct{}{}
		if input.Identities[index].Primary {
			primaryByType[normalized.Type]++
			if primaryByType[normalized.Type] > 1 {
				return fmt.Errorf("only one primary %s identity is allowed", normalized.Type)
			}
		}
		if input.Identities[index].Metadata != nil {
			encoded, err := json.Marshal(input.Identities[index].Metadata)
			if err != nil {
				return fmt.Errorf("identity %d metadata: %w", index, err)
			}
			if len(encoded) > 16*1024 {
				return fmt.Errorf("identity %d metadata exceeds 16 KiB", index)
			}
		}
	}
	if err := input.Profile.Validate(); err != nil {
		return err
	}
	if input.Tags != nil {
		normalizedTags := make([]string, 0, len(*input.Tags))
		seenTags := make(map[string]struct{}, len(*input.Tags))
		for _, rawTag := range *input.Tags {
			tag := trimUnicodeSpace(rawTag)
			if tag == "" || utf8.RuneCountInString(tag) > 64 {
				return fmt.Errorf("tag must contain 1 to 64 characters")
			}
			if _, exists := seenTags[tag]; exists {
				continue
			}
			seenTags[tag] = struct{}{}
			normalizedTags = append(normalizedTags, tag)
		}
		sort.Strings(normalizedTags)
		*input.Tags = normalizedTags
	}

	if input.ListMemberships != nil && input.ListMembershipsAdd != nil {
		return fmt.Errorf("list_memberships and list_memberships_add cannot be used together")
	}
	for fieldName, memberships := range map[string]*[]CustomerListMembershipInput{
		"list_memberships": input.ListMemberships, "list_memberships_add": input.ListMembershipsAdd,
	} {
		if memberships == nil {
			continue
		}
		seenLists := make(map[string]struct{}, len(*memberships))
		for index := range *memberships {
			membership := &(*memberships)[index]
			membership.ListID = trimUnicodeSpace(membership.ListID)
			if membership.ListID == "" || utf8.RuneCountInString(membership.ListID) > 32 || !govalidator.IsAlphanumeric(membership.ListID) {
				return fmt.Errorf("%s list_id must be alphanumeric and contain 1 to 32 characters", fieldName)
			}
			if _, exists := seenLists[membership.ListID]; exists {
				return fmt.Errorf("duplicate %s membership %s", fieldName, membership.ListID)
			}
			seenLists[membership.ListID] = struct{}{}
			membership.Status = strings.ToLower(trimUnicodeSpace(membership.Status))
			if membership.Status == "" {
				membership.Status = "active"
			}
			switch membership.Status {
			case "active", "pending", "unsubscribed", "bounced", "complained":
			default:
				return fmt.Errorf("list membership status is invalid")
			}
		}
	}
	return nil
}

func (input CustomerUpsertInput) CanonicalPayloadHash() (string, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode customer payload: %w", err)
	}
	var canonical CustomerUpsertInput
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return "", fmt.Errorf("copy customer payload: %w", err)
	}
	if err := canonical.Validate(); err != nil {
		return "", err
	}
	sort.Slice(canonical.Identities, func(i, j int) bool {
		left := string(canonical.Identities[i].Type) + "\x00" + canonical.Identities[i].Value
		right := string(canonical.Identities[j].Type) + "\x00" + canonical.Identities[j].Value
		return left < right
	})
	if canonical.ListMemberships != nil {
		sort.Slice(*canonical.ListMemberships, func(i, j int) bool {
			return (*canonical.ListMemberships)[i].ListID < (*canonical.ListMemberships)[j].ListID
		})
	}
	if canonical.ListMembershipsAdd != nil {
		sort.Slice(*canonical.ListMembershipsAdd, func(i, j int) bool {
			return (*canonical.ListMembershipsAdd)[i].ListID < (*canonical.ListMembershipsAdd)[j].ListID
		})
	}
	encoded, err = json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode canonical customer payload: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum[:]), nil
}

type UpsertCustomerRequest struct {
	WorkspaceID    string              `json:"workspace_id"`
	IdempotencyKey string              `json:"idempotency_key"`
	Customer       CustomerUpsertInput `json:"customer"`
}

func (request *UpsertCustomerRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || utf8.RuneCountInString(request.WorkspaceID) > 32 || !govalidator.IsAlphanumeric(request.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric and contain 1 to 32 characters")
	}
	request.IdempotencyKey = trimUnicodeSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" || utf8.RuneCountInString(request.IdempotencyKey) > 255 {
		return fmt.Errorf("idempotency_key must contain 1 to 255 characters")
	}
	return request.Customer.Validate()
}

type GetCustomerRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	Locator     CustomerLocator `json:"locator"`
}

const (
	DefaultCustomerListLimit = 50
	MaxCustomerListLimit     = 200
)

type CustomerListCursor struct {
	CreatedAt  time.Time `json:"created_at"`
	CustomerID string    `json:"customer_id"`
}

func EncodeCustomerListCursor(cursor CustomerListCursor) (string, error) {
	if cursor.CreatedAt.IsZero() {
		return "", fmt.Errorf("customer list cursor created_at is required")
	}
	parsed, err := uuid.Parse(strings.TrimSpace(cursor.CustomerID))
	if err != nil || parsed == uuid.Nil {
		return "", fmt.Errorf("customer list cursor customer_id must be a non-nil UUID")
	}
	cursor.CreatedAt = cursor.CreatedAt.UTC()
	cursor.CustomerID = parsed.String()
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode customer list cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func DecodeCustomerListCursor(encoded string) (CustomerListCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return CustomerListCursor{}, fmt.Errorf("invalid customer list cursor: %w", err)
	}
	var cursor CustomerListCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return CustomerListCursor{}, fmt.Errorf("invalid customer list cursor: %w", err)
	}
	canonical, err := EncodeCustomerListCursor(cursor)
	if err != nil {
		return CustomerListCursor{}, fmt.Errorf("invalid customer list cursor: %w", err)
	}
	if canonical != strings.TrimSpace(encoded) {
		return CustomerListCursor{}, fmt.Errorf("invalid customer list cursor encoding")
	}
	return cursor, nil
}

type CustomerListRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	Search        string `json:"search,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	IncludeMerged bool   `json:"include_merged,omitempty"`
}

func (request *CustomerListRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || utf8.RuneCountInString(request.WorkspaceID) > 32 || !govalidator.IsAlphanumeric(request.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric and contain 1 to 32 characters")
	}
	request.Search = trimUnicodeSpace(request.Search)
	if utf8.RuneCountInString(request.Search) > 255 {
		return fmt.Errorf("customer search cannot exceed 255 characters")
	}
	if request.Limit == 0 {
		request.Limit = DefaultCustomerListLimit
	}
	if request.Limit < 1 || request.Limit > MaxCustomerListLimit {
		return fmt.Errorf("customer list limit must be between 1 and %d", MaxCustomerListLimit)
	}
	if request.Cursor != "" {
		if _, err := DecodeCustomerListCursor(request.Cursor); err != nil {
			return err
		}
	}
	return nil
}

type CustomerSummary struct {
	ID             string             `json:"customer_id"`
	CustomerNo     string             `json:"customer_no"`
	ExternalUserID *string            `json:"external_user_id,omitempty"`
	MergedIntoID   *string            `json:"merged_into_id,omitempty"`
	Version        int64              `json:"version"`
	Profile        *CustomerProfile   `json:"profile,omitempty"`
	Identities     []CustomerIdentity `json:"identities"`
	Tags           []string           `json:"tags"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

type CustomerListResponse struct {
	Customers  []CustomerSummary `json:"customers"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (request *GetCustomerRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || utf8.RuneCountInString(request.WorkspaceID) > 32 || !govalidator.IsAlphanumeric(request.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric and contain 1 to 32 characters")
	}
	return request.Locator.Validate()
}

type CustomerBatchUpsertItem struct {
	IdempotencyKey string              `json:"idempotency_key"`
	Customer       CustomerUpsertInput `json:"customer"`
}

type CustomerBatchUpsertRequest struct {
	WorkspaceID string                    `json:"workspace_id"`
	Items       []CustomerBatchUpsertItem `json:"items"`
}

func (request *CustomerBatchUpsertRequest) Validate(maxItems int) error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || utf8.RuneCountInString(request.WorkspaceID) > 32 || !govalidator.IsAlphanumeric(request.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric and contain 1 to 32 characters")
	}
	if maxItems <= 0 {
		return fmt.Errorf("customer batch limit must be positive")
	}
	if len(request.Items) == 0 {
		return fmt.Errorf("customer batch must contain at least one item")
	}
	if len(request.Items) > maxItems {
		return fmt.Errorf("customer batch cannot exceed %d items", maxItems)
	}
	return nil
}

type CustomerBatchItemError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CustomerBatchItemResult struct {
	Index          int                     `json:"index"`
	IdempotencyKey string                  `json:"idempotency_key,omitempty"`
	Status         string                  `json:"status"`
	Customer       *CustomerMutationResult `json:"customer,omitempty"`
	Error          *CustomerBatchItemError `json:"error,omitempty"`
}

type CustomerBatchUpsertResponse struct {
	Accepted int                       `json:"accepted"`
	Failed   int                       `json:"failed"`
	Results  []CustomerBatchItemResult `json:"results"`
}

type CustomerMergeRequest struct {
	WorkspaceID    string          `json:"workspace_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Source         CustomerLocator `json:"source"`
	Target         CustomerLocator `json:"target"`
	Reason         string          `json:"reason,omitempty"`
}

func (request *CustomerMergeRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || utf8.RuneCountInString(request.WorkspaceID) > 32 || !govalidator.IsAlphanumeric(request.WorkspaceID) {
		return fmt.Errorf("workspace_id must be alphanumeric and contain 1 to 32 characters")
	}
	request.IdempotencyKey = trimUnicodeSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" || utf8.RuneCountInString(request.IdempotencyKey) > 255 {
		return fmt.Errorf("idempotency_key must contain 1 to 255 characters")
	}
	if err := request.Source.Validate(); err != nil {
		return fmt.Errorf("invalid source locator: %w", err)
	}
	if err := request.Target.Validate(); err != nil {
		return fmt.Errorf("invalid target locator: %w", err)
	}
	source, _ := json.Marshal(request.Source)
	target, _ := json.Marshal(request.Target)
	if string(source) == string(target) {
		return fmt.Errorf("source and target locators must be different")
	}
	request.Reason = trimUnicodeSpace(request.Reason)
	if utf8.RuneCountInString(request.Reason) > 500 {
		return fmt.Errorf("merge reason cannot exceed 500 characters")
	}
	return nil
}

func (request CustomerMergeRequest) CanonicalPayloadHash() (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Source CustomerLocator `json:"source"`
		Target CustomerLocator `json:"target"`
		Reason string          `json:"reason,omitempty"`
	}{Source: request.Source, Target: request.Target, Reason: request.Reason})
	if err != nil {
		return "", fmt.Errorf("encode canonical customer merge payload: %w", err)
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum[:]), nil
}

type CustomerMergeCommand struct {
	WorkspaceID    string
	IdempotencyKey string
	PayloadHash    string
	Source         CustomerLocator
	Target         CustomerLocator
	ActorID        string
	Reason         string
}

type CustomerMergeResult struct {
	SourceCustomerID string `json:"source_customer_id"`
	TargetCustomerID string `json:"target_customer_id"`
	TargetCustomerNo string `json:"target_customer_no"`
	TargetVersion    int64  `json:"target_version"`
	Replayed         bool   `json:"replayed"`
}

type CustomerService interface {
	GetCustomer(ctx context.Context, request *GetCustomerRequest) (*Customer, error)
	ListCustomers(ctx context.Context, request *CustomerListRequest) (*CustomerListResponse, error)
	UpsertCustomer(ctx context.Context, request *UpsertCustomerRequest) (*CustomerMutationResult, error)
	UpsertCustomerBatch(ctx context.Context, request *CustomerBatchUpsertRequest) (*CustomerBatchUpsertResponse, error)
	UpdateCustomerListMemberships(ctx context.Context, request *CustomerListMembershipUpdateRequest) (*CustomerListMembershipUpdateResult, error)
	MergeCustomer(ctx context.Context, request *CustomerMergeRequest) (*CustomerMergeResult, error)
}

//go:generate mockgen -destination mocks/mock_customer_service.go -package mocks github.com/hengshu-credit/yaoguang-marketing/internal/domain CustomerService

type Customer struct {
	ID                     string                       `json:"customer_id"`
	CustomerNo             string                       `json:"customer_no"`
	ExternalUserID         *string                      `json:"external_user_id,omitempty"`
	MergedIntoID           *string                      `json:"merged_into_id,omitempty"`
	ResolvedFromCustomerID *string                      `json:"resolved_from_customer_id,omitempty"`
	Version                int64                        `json:"version"`
	Profile                *CustomerProfile             `json:"profile,omitempty"`
	Identities             []CustomerIdentity           `json:"identities"`
	Tags                   []string                     `json:"tags"`
	ListMemberships        []CustomerListMembership     `json:"list_memberships"`
	AudienceMemberships    []CustomerAudienceMembership `json:"audience_memberships"`
	Consents               []CustomerConsent            `json:"consents"`
	CreatedAt              time.Time                    `json:"created_at"`
	UpdatedAt              time.Time                    `json:"updated_at"`
}

type CustomerIdentity struct {
	ID          string                 `json:"id"`
	Type        CustomerIdentityType   `json:"type"`
	DisplayHint string                 `json:"display_hint"`
	Verified    bool                   `json:"verified"`
	Primary     bool                   `json:"primary"`
	Enabled     bool                   `json:"enabled"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type CustomerListMembership struct {
	ListID    string    `json:"list_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CustomerAudienceMembership describes membership in the audience's current,
// completed build. Historical or stale builds are deliberately excluded from
// Customer 360 so business users do not act on obsolete membership data.
type CustomerAudienceMembership struct {
	AudienceID      string    `json:"audience_id"`
	Name            string    `json:"name"`
	Kind            string    `json:"kind"`
	AudienceVersion int       `json:"audience_version"`
	BuildID         string    `json:"build_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type CustomerConsent struct {
	ID        string                 `json:"id"`
	Purpose   string                 `json:"purpose"`
	Channel   string                 `json:"channel"`
	Status    string                 `json:"status"`
	Source    *string                `json:"source,omitempty"`
	ValidFrom time.Time              `json:"valid_from"`
	RevokedAt *time.Time             `json:"revoked_at,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type CustomerMutationResult struct {
	CustomerID     string  `json:"customer_id"`
	CustomerNo     string  `json:"customer_no"`
	ExternalUserID *string `json:"external_user_id,omitempty"`
	Action         string  `json:"action"`
	Version        int64   `json:"version"`
	Replayed       bool    `json:"replayed"`
}

type CustomerUpsertCommand struct {
	WorkspaceID       string
	WorkspaceSequence uint16
	IdempotencyKey    string
	PayloadHash       string
	Input             CustomerUpsertInput
}

type CustomerRepository interface {
	Upsert(ctx context.Context, command CustomerUpsertCommand) (*CustomerMutationResult, error)
	Get(ctx context.Context, workspaceID string, locator CustomerLocator) (*Customer, error)
	List(ctx context.Context, workspaceID string, request CustomerListRequest) (*CustomerListResponse, error)
	UpdateListMemberships(ctx context.Context, request CustomerListMembershipUpdateRequest) (*CustomerListMembershipUpdateResult, error)
	Merge(ctx context.Context, command CustomerMergeCommand) (*CustomerMergeResult, error)
}

//go:generate mockgen -destination mocks/mock_customer_repository.go -package mocks github.com/hengshu-credit/yaoguang-marketing/internal/domain CustomerRepository

type ErrCustomerNotFound struct{}

func (*ErrCustomerNotFound) Error() string { return "customer not found" }

type ErrCustomerIdentityConflict struct {
	IdentityType CustomerIdentityType
}

func (e *ErrCustomerIdentityConflict) Error() string {
	return fmt.Sprintf("%s identity belongs to another customer", e.IdentityType)
}

type ErrCustomerExternalIDConflict struct{}

func (*ErrCustomerExternalIDConflict) Error() string {
	return "external_user_id belongs to another customer"
}

type ErrCustomerIdempotencyConflict struct{}

func (*ErrCustomerIdempotencyConflict) Error() string {
	return "idempotency key was already used with a different payload"
}

type ErrCustomerNumberConflict struct{}

func (*ErrCustomerNumberConflict) Error() string { return "customer number already exists" }

type ErrCustomerConflict struct {
	Constraint string
}

func (e *ErrCustomerConflict) Error() string {
	if e.Constraint == "" {
		return "customer mutation conflicts with existing data"
	}
	return "customer mutation conflicts with existing data: " + e.Constraint
}

type ErrCustomerMergeRejected struct {
	Reason string
}

func (e *ErrCustomerMergeRejected) Error() string { return "customer merge rejected: " + e.Reason }

func ApplyCustomerAttributesPatch(current map[string]interface{}, patch *CustomerAttributesPatch) (map[string]interface{}, error) {
	result, err := cloneCustomerAttributes(current)
	if err != nil {
		return nil, err
	}
	if patch == nil {
		return result, nil
	}

	if len(patch.Set) > 0 {
		result, err = decodeCustomerAttributeObject("set", patch.Set)
		if err != nil {
			return nil, err
		}
	}
	if len(patch.Merge) > 0 {
		merge, err := decodeCustomerAttributeObject("merge", patch.Merge)
		if err != nil {
			return nil, err
		}
		deepMergeCustomerAttributes(result, merge)
	}
	for _, path := range patch.Unset {
		parts := strings.Split(path, ".")
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				return nil, fmt.Errorf("unset path %q contains an empty segment", path)
			}
		}
		unsetCustomerAttributePath(result, parts)
	}
	return result, nil
}

func cloneCustomerAttributes(attributes map[string]interface{}) (map[string]interface{}, error) {
	if attributes == nil {
		return map[string]interface{}{}, nil
	}
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return nil, fmt.Errorf("encode current customer attributes: %w", err)
	}
	return decodeCustomerAttributeObject("current attributes", encoded)
}

func decodeCustomerAttributeObject(operation string, raw []byte) (map[string]interface{}, error) {
	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", operation)
	}
	return object, nil
}

func deepMergeCustomerAttributes(target, incoming map[string]interface{}) {
	for key, incomingValue := range incoming {
		incomingObject, incomingIsObject := incomingValue.(map[string]interface{})
		targetObject, targetIsObject := target[key].(map[string]interface{})
		if incomingIsObject && targetIsObject {
			deepMergeCustomerAttributes(targetObject, incomingObject)
			continue
		}
		target[key] = incomingValue
	}
}

func unsetCustomerAttributePath(attributes map[string]interface{}, path []string) {
	current := attributes
	for _, part := range path[:len(path)-1] {
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return
		}
		current = next
	}
	delete(current, path[len(path)-1])
}

// GenerateCustomerNumber creates the immutable business-facing Customer number.
// The internal UUID remains the relational and runtime authority.
func GenerateCustomerNumber(workspaceSequence uint16, at time.Time, customerID uuid.UUID) (string, error) {
	return GenerateCustomerNumberWithSuffixOffset(workspaceSequence, at, customerID, 0)
}

// GenerateCustomerNumberWithSuffixOffset creates a deterministic alternative
// for the rare case where two existing customers map to the same short suffix.
func GenerateCustomerNumberWithSuffixOffset(workspaceSequence uint16, at time.Time, customerID uuid.UUID, suffixOffset uint64) (string, error) {
	if workspaceSequence < MinWorkspaceSequence || workspaceSequence > MaxWorkspaceSequence {
		return "", fmt.Errorf("workspace sequence must be between %04d and %04d", MinWorkspaceSequence, MaxWorkspaceSequence)
	}
	if customerID == uuid.Nil {
		return "", fmt.Errorf("customer ID must not be nil")
	}

	var workspaceCode string
	if workspaceSequence <= 999 {
		workspaceCode = fmt.Sprintf("%03d", workspaceSequence)
	} else {
		extension := uint64(workspaceSequence - 1000)
		extensionSuffix := strconv.FormatUint(extension%(36*36), 36)
		workspaceCode = string(rune('a')+rune(extension/(36*36))) + strings.Repeat("0", 2-len(extensionSuffix)) + extensionSuffix
	}

	// The UUID remains the internal authority and entropy source. Folding all
	// 128 bits keeps the public identifier short without exposing the UUID.
	var suffixValue uint64
	for _, value := range customerID {
		suffixValue = (suffixValue*256 + uint64(value)) % customerNumberRandomSpace
	}
	suffixValue = (suffixValue + suffixOffset%customerNumberRandomSpace) % customerNumberRandomSpace
	randomSuffix := strconv.FormatUint(suffixValue, 36)
	randomSuffix = strings.Repeat("0", 6-len(randomSuffix)) + randomSuffix
	return fmt.Sprintf(
		"U%s%s08%s",
		workspaceCode,
		at.In(yaoguangCustomerNumberLocation).Format("20060102150405"),
		randomSuffix,
	), nil
}
