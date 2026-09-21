package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// BusinessConfigurationError identifies the control that needs correction.
type BusinessConfigurationError struct {
	Field   string
	Message string
}

func (e *BusinessConfigurationError) Error() string { return e.Field + ": " + e.Message }
func (e *BusinessConfigurationError) Unwrap() error { return NewValidationError(e.Error()) }

func reportingConfigError(field, message string) error {
	return &BusinessConfigurationError{Field: field, Message: message}
}

// IsBusinessObjectPath matches the literal object-key paths BusinessJSONPath
// actually supports. Arrays, wildcards and quoted JSONPath expressions are not supported.
func IsBusinessObjectPath(path string) bool {
	if path == "$" {
		return true
	}
	if !strings.HasPrefix(path, "$.") || len(path) > 1024 {
		return false
	}
	parts := strings.Split(path[2:], ".")
	if len(parts) > 16 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, "[]*\"' \t\r\n") || part == "__proto__" || part == "constructor" || part == "prototype" {
			return false
		}
	}
	return true
}

func isNormalizedAttributePath(path string) bool {
	return IsBusinessObjectPath(path) && (strings.HasPrefix(path, "$.data.") || strings.HasPrefix(path, "$.customer_attributes."))
}

// IsExactBrowserOrigin rejects URLs that cannot equal a browser's Origin header.
func IsExactBrowserOrigin(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.HasSuffix(u.Host, ":") || strings.ContainsAny(value, "*# \t\r\n") {
		return false
	}
	if value != u.Scheme+"://"+strings.ToLower(u.Host) {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || (u.Scheme == "https" && n == 443) || (u.Scheme == "http" && n == 80) {
			return false
		}
	}
	return true
}

// ValidateReportingConfiguration allows incomplete drafts, but prevents invalid
// active configurations. Missing expiry_mode keeps the legacy "required" meaning.
func (d *BusinessDefinition) ValidateReportingConfiguration(complete bool) error {
	if d.Kind != "event" && d.Kind != "event_mapping" {
		return nil
	}
	// Retiring an existing invalid event must remain possible after validation is tightened.
	if d.Status == "archived" && !complete {
		return nil
	}
	s := d.Settings
	if d.Kind == "event_mapping" {
		fields, ok := s["fields"].(map[string]any)
		if !ok {
			return reportingConfigError("settings.fields", "mapping fields must be an object")
		}
		for _, key := range []string{"report_id", "event_code", "event_name", "customer_attributes", "data", "triggered_at", "expires_at", "backfill"} {
			if raw, exists := fields[key]; exists {
				path, ok := raw.(string)
				if !ok || !IsBusinessObjectPath(path) {
					return reportingConfigError("settings.fields."+key, "use a literal JSON object path, or remove this optional mapping")
				}
			}
		}
		if code, exists := s["event_code"]; exists && code != "" {
			text, ok := code.(string)
			if !ok || !businessKeyPattern.MatchString(text) {
				return reportingConfigError("settings.event_code", "use a stable event code")
			}
		}
		if complete {
			for _, key := range []string{"report_id", "data", "triggered_at"} {
				if _, exists := fields[key]; !exists {
					return reportingConfigError("settings.fields."+key, "a mapping is required before activation or preview")
				}
			}
			if s["event_code"] == nil || s["event_code"] == "" {
				if _, exists := fields["event_code"]; !exists {
					return reportingConfigError("settings.fields.event_code", "map the event code or select a fixed event")
				}
			}
			if _, exists := fields["customer_attributes"]; !exists {
				var rules []AttributeMapping
				encoded, _ := json.Marshal(s["attribute_mappings"])
				_ = json.Unmarshal(encoded, &rules)
				hasIdentity := false
				for _, rule := range rules {
					if rule.Action == "lookup" || rule.Action == "bind" {
						hasIdentity = true
					}
				}
				if !hasIdentity {
					return reportingConfigError("settings.fields.customer_attributes", "map customer attributes or add a lookup/bind attribute mapping")
				}
			}
		}
		return nil
	}
	for _, key := range []string{"browser_enabled", "authoritative"} {
		if value, exists := s[key]; exists {
			if _, ok := value.(bool); !ok {
				return reportingConfigError("settings."+key, "must be a boolean")
			}
		}
	}
	for _, key := range []string{"expiry_mode", "unmatched_customer", "subject_type"} {
		if value, exists := s[key]; exists {
			if _, ok := value.(string); !ok {
				return reportingConfigError("settings."+key, "must be text")
			}
		}
	}
	mode, _ := s["expiry_mode"].(string)
	switch mode {
	case "", "never", "required":
	case "duration":
		if s["ttl_seconds"] != nil || complete {
			seconds, ok := businessNumber(s["ttl_seconds"])
			if !ok || math.IsNaN(seconds) || math.IsInf(seconds, 0) || math.Trunc(seconds) != seconds || seconds < 1 || seconds > 315360000 {
				return reportingConfigError("settings.ttl_seconds", "use a whole number of seconds between 1 and 315360000")
			}
		}
	default:
		return reportingConfigError("settings.expiry_mode", "choose never, duration or required")
	}
	switch s["unmatched_customer"] {
	case nil, "", "pending", "reject", "create":
	default:
		return reportingConfigError("settings.unmatched_customer", "choose pending, reject or create")
	}
	subject, _ := s["subject_type"].(string)
	if subject != "" && subject != "customer" {
		if !businessKeyPattern.MatchString(subject) {
			return reportingConfigError("settings.subject_type", "use a configured object type")
		}
		path, _ := s["subject_id_path"].(string)
		if (path != "" || complete) && !isNormalizedAttributePath(path) {
			return reportingConfigError("settings.subject_id_path", "select an object ID under $.data or $.customer_attributes")
		}
		if path, exists := s["subject_attributes_path"]; exists {
			text, ok := path.(string)
			if !ok || !IsBusinessObjectPath(text) || !(text == "$.data" || text == "$.customer_attributes" || isNormalizedAttributePath(text)) {
				return reportingConfigError("settings.subject_attributes_path", "select an object under $.data or $.customer_attributes")
			}
		}
	}
	if raw, exists := s["allowed_sources"]; exists {
		values, ok := raw.([]any)
		if !ok {
			return reportingConfigError("settings.allowed_sources", "must be an array of source identifiers")
		}
		if complete && len(values) == 0 {
			return reportingConfigError("settings.allowed_sources", "select a source, or remove the restriction to allow all authenticated sources")
		}
		for _, value := range values {
			if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" || len(text) > 128 || text != strings.TrimSpace(text) {
				return reportingConfigError("settings.allowed_sources", "source identifiers must be nonempty without surrounding whitespace")
			}
		}
	}
	if s["browser_enabled"] == true {
		if s["authoritative"] == true {
			return reportingConfigError("settings.browser_enabled", "browser reporting and trusted-backend-only reporting cannot both be enabled")
		}
		if subject != "" && subject != "customer" {
			return reportingConfigError("settings.subject_type", "browser reports can only describe a customer, not a business object")
		}
		values, ok := s["allowed_browser_origins"].([]any)
		if complete && (!ok || len(values) == 0) {
			return reportingConfigError("settings.allowed_browser_origins", "at least one exact HTTP origin is required")
		}
		if raw, exists := s["allowed_browser_origins"]; exists && !ok {
			return reportingConfigError("settings.allowed_browser_origins", fmt.Sprintf("expected an array, got %T", raw))
		}
		for _, value := range values {
			if text, ok := value.(string); !ok || !IsExactBrowserOrigin(text) {
				return reportingConfigError("settings.allowed_browser_origins", "use exact origins such as https://app.example.com, without paths, wildcards or default ports")
			}
		}
	}
	return nil
}
