package domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
)

const (
	MinWorkspaceSequence uint16 = 1
	MaxWorkspaceSequence uint16 = 9999
)

var yaoguangCustomerNumberLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

var (
	customerNumberPattern = regexp.MustCompile(`^U[0-9]{4}[0-9]{14}08[0-9a-f]{32}$`)
	customerE164Pattern   = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
)

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

type CustomerUpsertInput struct {
	Locator         *CustomerLocator              `json:"locator,omitempty"`
	ExternalUserID  *string                       `json:"external_user_id,omitempty"`
	Profile         *CustomerProfilePatch         `json:"profile,omitempty"`
	Identities      []CustomerIdentityInput       `json:"identities,omitempty"`
	Tags            *[]string                     `json:"tags,omitempty"`
	ListMemberships []CustomerListMembershipInput `json:"list_memberships,omitempty"`
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

	seenLists := make(map[string]struct{}, len(input.ListMemberships))
	for index := range input.ListMemberships {
		membership := &input.ListMemberships[index]
		membership.ListID = trimUnicodeSpace(membership.ListID)
		if membership.ListID == "" || utf8.RuneCountInString(membership.ListID) > 32 || !govalidator.IsAlphanumeric(membership.ListID) {
			return fmt.Errorf("list_id must be alphanumeric and contain 1 to 32 characters")
		}
		if _, exists := seenLists[membership.ListID]; exists {
			return fmt.Errorf("duplicate list membership %s", membership.ListID)
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
	sort.Slice(canonical.ListMemberships, func(i, j int) bool {
		return canonical.ListMemberships[i].ListID < canonical.ListMemberships[j].ListID
	})
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

type Customer struct {
	ID                     string                   `json:"customer_id"`
	CustomerNo             string                   `json:"customer_no"`
	ExternalUserID         *string                  `json:"external_user_id,omitempty"`
	MergedIntoID           *string                  `json:"merged_into_id,omitempty"`
	ResolvedFromCustomerID *string                  `json:"resolved_from_customer_id,omitempty"`
	Version                int64                    `json:"version"`
	Profile                *CustomerProfile         `json:"profile,omitempty"`
	Identities             []CustomerIdentity       `json:"identities"`
	Tags                   []string                 `json:"tags"`
	ListMemberships        []CustomerListMembership `json:"list_memberships"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
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
}

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
	if workspaceSequence < MinWorkspaceSequence || workspaceSequence > MaxWorkspaceSequence {
		return "", fmt.Errorf("workspace sequence must be between %04d and %04d", MinWorkspaceSequence, MaxWorkspaceSequence)
	}
	if customerID == uuid.Nil {
		return "", fmt.Errorf("customer ID must not be nil")
	}

	uuidSuffix := strings.ReplaceAll(strings.ToLower(customerID.String()), "-", "")
	return fmt.Sprintf(
		"U%04d%s08%s",
		workspaceSequence,
		at.In(yaoguangCustomerNumberLocation).Format("20060102150405"),
		uuidSuffix,
	), nil
}
