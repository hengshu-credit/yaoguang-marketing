package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// ExtendEventSchema adds optional fields without changing existing producers.
func ExtendEventSchema(existing, extension map[string]any) (map[string]any, error) {
	if extension["type"] != "object" {
		return nil, NewValidationError("event fields must be an object schema")
	}
	if err := validateBusinessSchema(extension, 0); err != nil {
		return nil, NewValidationError(err.Error())
	}
	required, _ := json.Marshal(extension["required"])
	if string(required) != "null" && string(required) != "[]" {
		return nil, NewValidationError("shared event fields must be optional for existing producers")
	}
	properties, _ := extension["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil, NewValidationError("add at least one event field")
	}
	raw, err := json.Marshal(existing)
	if err != nil {
		return nil, err
	}
	merged := map[string]any{}
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, err
	}
	current, _ := merged["properties"].(map[string]any)
	if current == nil {
		current = map[string]any{}
	}
	for key, field := range properties {
		if previous, ok := current[key]; ok && !reflect.DeepEqual(previous, field) {
			return nil, NewValidationError(fmt.Sprintf("event field %s already exists with a different definition", key))
		}
		current[key] = field
	}
	merged["type"], merged["properties"] = "object", current
	return merged, nil
}
