package service

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"strings"
)

// Read access to contacts does not imply access to sensitive configured attributes.
func (s *TrackedEventService) redactResults(ctx context.Context, w string, results []domain.TrackedEventResult) ([]domain.TrackedEventResult, error) {
	_, _, member, err := s.auth.AuthenticateUserForWorkspace(ctx, w)
	if err != nil {
		return nil, err
	}
	if member != nil && member.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite) {
		return results, nil
	}
	definitions, _, err := s.definitions.List(ctx, w, domain.BusinessDefinitionFilter{Kind: "customer_attribute", Limit: 1000})
	if err != nil {
		return nil, err
	}
	sensitive := map[string]bool{}
	for _, d := range definitions {
		if d.Settings["sensitive"] == true {
			sensitive[d.Key] = true
		}
	}
	// Redact names recursively, including matching business-data copies; values never leave this boundary.
	var redact func(any)
	redact = func(value any) {
		switch object := value.(type) {
		case map[string]any:
			for key, value := range object {
				if sensitive[key] {
					object[key] = "[redacted]"
					continue
				}
				redact(value)
			}
		case []any:
			for _, entry := range object {
				redact(entry)
			}
		}
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}
	var safe []domain.TrackedEventResult
	if err = json.Unmarshal(encoded, &safe); err != nil {
		return nil, err
	}
	for i := range safe {
		if safe[i].Event != nil {
			for _, key := range safe[i].Event.SensitiveAttributes {
				sensitive[key] = true
			}
			root := map[string]any{"customer_attributes": safe[i].Event.CustomerAttributes, "data": safe[i].Event.Data}
			for key, paths := range safe[i].Event.AttributePaths {
				if !sensitive[key] {
					continue
				}
				for _, path := range paths {
					if !strings.HasPrefix(path, "$.") {
						continue
					}
					parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
					object := root
					for _, part := range parts[:len(parts)-1] {
						object, _ = object[part].(map[string]any)
						if object == nil {
							break
						}
					}
					if object != nil {
						if _, exists := object[parts[len(parts)-1]]; exists {
							object[parts[len(parts)-1]] = "[redacted]"
						}
					}
				}
			}
			redact(safe[i].Event.CustomerAttributes)
			redact(safe[i].Event.Data)
		}
	}
	return safe, nil
}
