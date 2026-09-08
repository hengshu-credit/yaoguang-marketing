package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (s *ImportJobService) FileFields(ctx context.Context, workspaceID string) ([]domain.ContactFileField, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	includeProfiles := true
	if s.auth != nil {
		authorized, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if membership == nil || (!membership.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) && !membership.HasPermission(domain.PermissionResourceCustomers, domain.PermissionTypeRead)) {
			return nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Contact file fields require read permission")
		}
		ctx = authorized
		includeProfiles = membership.HasPermission(domain.PermissionResourceCustomers, domain.PermissionTypeRead)
	}
	fields, err := s.loadFileFields(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if includeProfiles {
		return fields, nil
	}
	visible := make([]domain.ContactFileField, 0, len(fields))
	for _, field := range fields {
		if !strings.HasPrefix(field.Key, "profile.") {
			visible = append(visible, field)
		}
	}
	return visible, nil
}

func (s *ImportJobService) loadFileFields(ctx context.Context, workspaceID string) ([]domain.ContactFileField, error) {
	if s.fileFields != nil {
		return s.fileFields.Fields(ctx, workspaceID)
	}
	return domain.BaseContactFileFields(nil), nil
}

func resolveContactFileHeaders(headers []string, fields []domain.ContactFileField) bool {
	aliases := map[string]string{"external_user_id": "external_id"}
	for _, field := range fields {
		aliases[field.Header] = field.Key
		if start := strings.LastIndex(field.Header, " ("); start >= 0 {
			plain := field.Header[:start]
			if previous, exists := aliases[plain]; exists && previous != field.Key {
				aliases[plain] = ""
			} else if !exists {
				aliases[plain] = field.Key
			}
		}
	}
	for _, field := range fields {
		aliases[field.Key] = field.Key
	}
	folded := map[string]string{}
	for alias, key := range aliases {
		lower := strings.ToLower(alias)
		if previous, ok := folded[lower]; ok && previous != key {
			folded[lower] = ""
		} else if !ok {
			folded[lower] = key
		}
	}
	seen := map[string]bool{}
	hasIdentity := false
	for index, header := range headers {
		header = strings.TrimSpace(header)
		key := aliases[header]
		if key == "" && strings.HasSuffix(header, ")") {
			if start := strings.LastIndex(header, " ("); start >= 0 {
				key = aliases[header[start+2:len(header)-1]]
			}
		}
		if key == "" {
			key = folded[strings.ToLower(header)]
		}
		if key == "" || seen[key] {
			return false
		}
		headers[index], seen[key] = key, true
		hasIdentity = hasIdentity || key == "external_id" || key == "email" || key == "phone"
	}
	return hasIdentity
}

func parseContactFileValue(raw, kind string) (interface{}, error) {
	switch kind {
	case "number", "integer":
		value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || (kind == "integer" && math.Trunc(value) != value) {
			return nil, fmt.Errorf("invalid %s value", kind)
		}
		return value, nil
	case "boolean":
		return strconv.ParseBool(strings.TrimSpace(raw))
	case "json", "auto":
		var value interface{}
		err := json.Unmarshal([]byte(raw), &value)
		if err != nil && kind == "auto" {
			return raw, nil
		}
		return value, err
	default:
		return raw, nil
	}
}

// Materialize dynamic types at staging time so a later definition change
// cannot reinterpret a queued import.
func normalizeContactFilePayload(payload []byte, fields []domain.ContactFileField) ([]byte, error) {
	values := map[string]string{}
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, err
	}
	attributes := map[string]interface{}{}
	for _, field := range fields {
		raw, exists := values[field.Key]
		if !exists {
			continue
		}
		if field.ReadOnly {
			delete(values, field.Key)
			continue
		}
		if !strings.HasPrefix(field.Key, "profile.attributes.") {
			continue
		}
		delete(values, field.Key)
		if raw == "" {
			continue
		}
		value, err := parseContactFileValue(raw, field.Type)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Header, err)
		}
		attributes[strings.TrimPrefix(field.Key, "profile.attributes.")] = value
	}
	if len(attributes) > 0 {
		encoded, err := json.Marshal(attributes)
		if err != nil {
			return nil, err
		}
		values["profile.attributes"] = string(encoded)
	}
	return json.Marshal(values)
}
