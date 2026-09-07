package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"math"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrBusinessDefinitionConflict = errors.New("business definition revision or key conflict")
var ErrBusinessDefinitionNotFound = errors.New("business definition not found")

type BusinessDefinition struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	Settings    map[string]any `json:"settings"`
	Status      string         `json:"status"`
	Revision    int            `json:"revision"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
type BusinessDefinitionFilter struct {
	Search string `json:"search"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}
type BusinessDefinitionRepository interface {
	List(context.Context, string, BusinessDefinitionFilter) ([]BusinessDefinition, int, error)
	Get(context.Context, string, string, string) (*BusinessDefinition, error)
	Save(context.Context, string, *BusinessDefinition) (*BusinessDefinition, error)
}

var businessKeyPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{0,127}$`)

func (d *BusinessDefinition) Validate() error {
	switch d.Kind {
	case "customer_attribute", "object_type", "object_relation", "event", "event_mapping", "state", "funnel":
	default:
		return fmt.Errorf("unsupported definition kind %q", d.Kind)
	}
	if !businessKeyPattern.MatchString(d.Key) || d.Name == "" {
		return fmt.Errorf("key and name are required; key must be a stable identifier")
	}
	if d.Status == "" {
		d.Status = "draft"
	}
	switch d.Status {
	case "draft", "active", "archived":
	default:
		return fmt.Errorf("invalid definition status")
	}
	if d.Schema == nil {
		d.Schema = map[string]any{}
	}
	if d.Settings == nil {
		d.Settings = map[string]any{}
	}
	if err := validateBusinessSchema(d.Schema, 0); err != nil {
		return err
	}
	if d.Kind == "object_relation" {
		from, _ := d.Settings["from_type"].(string)
		to, _ := d.Settings["to_type"].(string)
		card, _ := d.Settings["cardinality"].(string)
		if from == "" || to == "" || from == to {
			return fmt.Errorf("relation requires distinct from_type and to_type")
		}
		if card != "one_to_one" && card != "one_to_many" && card != "many_to_many" {
			return fmt.Errorf("invalid relation cardinality")
		}
	}
	if d.Kind == "event_mapping" {
		fields, ok := d.Settings["fields"].(map[string]any)
		if !ok {
			return fmt.Errorf("mapping fields must be an object")
		}
		for key, value := range fields {
			switch key {
			case "report_id", "event_code", "event_name", "customer_attributes", "data", "triggered_at", "expires_at", "backfill":
			default:
				return fmt.Errorf("unsupported normalized field %q", key)
			}
			path, ok := value.(string)
			if !ok || (!strings.HasPrefix(path, "$.") && path != "$") {
				return fmt.Errorf("mapping %s must use a JSON object path", key)
			}
		}
		if raw, exists := d.Settings["attribute_mappings"]; exists {
			encoded, _ := json.Marshal(raw)
			var mappings []AttributeMapping
			if err := json.Unmarshal(encoded, &mappings); err != nil {
				return fmt.Errorf("attribute_mappings must be an array")
			}
			for _, m := range mappings {
				if m.Attribute == "" || !strings.HasPrefix(m.Path, "$.") {
					return fmt.Errorf("attribute mappings require attribute and path")
				}
				switch m.Action {
				case "lookup", "bind", "update", "record":
				default:
					return fmt.Errorf("invalid attribute mapping action")
				}
				if m.NullPolicy != "" && m.NullPolicy != "ignore" && m.NullPolicy != "clear" {
					return fmt.Errorf("invalid null policy")
				}
				if m.UpdatePolicy != "" && m.UpdatePolicy != "overwrite" && m.UpdatePolicy != "set_union" {
					return fmt.Errorf("invalid update policy")
				}
			}
		}
	}
	return nil
}
func validateBusinessSchema(s map[string]any, depth int) error {
	if depth > 16 {
		return fmt.Errorf("schema nesting exceeds 16")
	}
	for keyword := range s {
		switch keyword {
		case "type", "properties", "required", "items", "additionalProperties", "enum", "minimum", "maximum", "minLength", "maxLength", "format", "description", "title", "default":
		default:
			return fmt.Errorf("unsupported schema keyword %q", keyword)
		}
	}
	if raw, ok := s["type"]; ok {
		types := []any{raw}
		if list, ok := raw.([]any); ok {
			types = list
		}
		if len(types) == 0 {
			return fmt.Errorf("schema type union must not be empty")
		}
		for _, entry := range types {
			typ, ok := entry.(string)
			if !ok {
				return fmt.Errorf("schema types must be strings")
			}
			switch typ {
			case "string", "number", "integer", "boolean", "object", "array", "null":
			default:
				return fmt.Errorf("unsupported schema type %q", typ)
			}
		}
	}
	if raw, ok := s["required"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("required must be an array")
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("required entries must be strings")
			}
		}
	}
	if raw, ok := s["format"]; ok {
		switch raw {
		case "date-time", "email", "uuid":
		default:
			return fmt.Errorf("unsupported schema format %v", raw)
		}
	}
	if raw, ok := s["properties"]; ok {
		props, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("schema properties must be an object")
		}
		for _, v := range props {
			child, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("property schema must be an object")
			}
			if err := validateBusinessSchema(child, depth+1); err != nil {
				return err
			}
		}
	}
	if raw, ok := s["items"]; ok {
		child, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("items must be a schema object")
		}
		return validateBusinessSchema(child, depth+1)
	}
	return nil
}

// ValidateBusinessValue validates the supported, deliberately bounded JSON schema subset.
func ValidateBusinessValue(s map[string]any, v any, path string) error {
	if union, ok := s["type"].([]any); ok {
		for _, entry := range union {
			copy := map[string]any{}
			for k, x := range s {
				copy[k] = x
			}
			copy["type"] = entry
			if ValidateBusinessValue(copy, v, path) == nil {
				return nil
			}
		}
		return fmt.Errorf("%s does not match permitted types", path)
	}
	typ, _ := s["type"].(string)
	valid := true
	switch typ {
	case "string":
		_, valid = v.(string)
	case "boolean":
		_, valid = v.(bool)
	case "number", "integer":
		var n float64
		switch x := v.(type) {
		case float64:
			n = x
		case int:
			n = float64(x)
		case json.Number:
			var err error
			n, err = x.Float64()
			valid = err == nil
		default:
			valid = false
		}
		if typ == "integer" && n != math.Trunc(n) {
			valid = false
		}
	case "object":
		_, valid = v.(map[string]any)
	case "array":
		_, valid = v.([]any)
	case "null":
		valid = v == nil
	}
	if !valid {
		return fmt.Errorf("%s must be %s", path, typ)
	}
	if str, ok := v.(string); ok {
		length := float64(utf8.RuneCountInString(str))
		if min, ok := businessNumber(s["minLength"]); ok && length < min {
			return fmt.Errorf("%s is too short", path)
		}
		if max, ok := businessNumber(s["maxLength"]); ok && length > max {
			return fmt.Errorf("%s is too long", path)
		}
		switch s["format"] {
		case "date-time":
			if _, err := time.Parse(time.RFC3339Nano, str); err != nil {
				return fmt.Errorf("%s must be RFC3339 date-time", path)
			}
		case "email":
			parsed, err := mail.ParseAddress(str)
			if err != nil || parsed.Address != str {
				return fmt.Errorf("%s must be an email", path)
			}
		case "uuid":
			if _, err := uuid.Parse(str); err != nil {
				return fmt.Errorf("%s must be a UUID", path)
			}
		}
	}
	if n, ok := businessNumber(v); ok {
		if min, ok := businessNumber(s["minimum"]); ok && n < min {
			return fmt.Errorf("%s is below minimum", path)
		}
		if max, ok := businessNumber(s["maximum"]); ok && n > max {
			return fmt.Errorf("%s exceeds maximum", path)
		}
	}
	if values, ok := s["enum"].([]any); ok {
		found := false
		a, _ := json.Marshal(v)
		for _, candidate := range values {
			b, _ := json.Marshal(candidate)
			if string(a) == string(b) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s is not an allowed value", path)
		}
	}
	if obj, ok := v.(map[string]any); ok {
		required, _ := s["required"].([]any)
		for _, field := range required {
			key, ok := field.(string)
			if !ok {
				return fmt.Errorf("required entries must be strings")
			}
			if _, exists := obj[key]; !exists {
				return fmt.Errorf("%s.%s is required", path, key)
			}
		}
		props, _ := s["properties"].(map[string]any)
		for key, value := range obj {
			if raw, exists := props[key]; exists {
				if child, ok := raw.(map[string]any); ok {
					if err := ValidateBusinessValue(child, value, path+"."+key); err != nil {
						return err
					}
				}
			} else if additional, ok := s["additionalProperties"].(bool); ok && !additional {
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
		}
	}
	if items, ok := v.([]any); ok {
		if child, ok := s["items"].(map[string]any); ok {
			for i, item := range items {
				if err := ValidateBusinessValue(child, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func businessNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		n, err := x.Float64()
		return n, err == nil
	}
	return 0, false
}
