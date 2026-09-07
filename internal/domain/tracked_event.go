package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrTrackedEventErased = errors.New("event or browser session was erased")

type AttributeMapping struct {
	Attribute    string `json:"attribute"`
	Path         string `json:"path"`
	Action       string `json:"action"`
	NullPolicy   string `json:"null_policy"`
	UpdatePolicy string `json:"update_policy"`
}
type TrackedEvent struct {
	AttributePaths      map[string][]string `json:"attribute_paths,omitempty"`
	SensitiveAttributes []string            `json:"sensitive_attributes,omitempty"`
	TrustedTouchpointID string              `json:"trusted_touchpoint_id,omitempty"`
	ReportID            string              `json:"report_id"`
	EventCode           string              `json:"event_code"`
	EventName           string              `json:"event_name"`
	CustomerAttributes  map[string]any      `json:"customer_attributes"`
	Data                map[string]any      `json:"data"`
	ReportedAt          time.Time           `json:"reported_at"`
	TriggeredAt         time.Time           `json:"triggered_at"`
	ExpiresAt           *time.Time          `json:"expires_at"`
	DefinitionRevision  int                 `json:"definition_revision"`
	MappingRevision     int                 `json:"mapping_revision"`
	ProcessingStatus    string              `json:"processing_status"`
	Backfill            bool                `json:"backfill"`
}
type TrackEventRequest struct {
	TouchpointToken string         `json:"touchpoint_token,omitempty"`
	WorkspaceID     string         `json:"workspace_id"`
	Source          string         `json:"source"`
	MappingKey      string         `json:"mapping_key,omitempty"`
	Event           map[string]any `json:"event"`
}
type TrackedEventResult struct {
	ReportID         string        `json:"report_id"`
	EventID          string        `json:"event_id"`
	CustomerID       string        `json:"customer_id,omitempty"`
	ReportedAt       time.Time     `json:"reported_at"`
	Duplicate        bool          `json:"duplicate"`
	ProcessingStatus string        `json:"processing_status"`
	Event            *TrackedEvent `json:"event,omitempty"`
}
type TrackedEventCommand struct {
	ObjectDefinition  *BusinessDefinition
	WorkspaceID       string
	WorkspaceSequence uint16
	Source            string
	PayloadHash       string
	Event             *TrackedEvent
	Definition        *BusinessDefinition
	Mapping           *BusinessDefinition
	Attributes        map[string]BusinessDefinition
}
type TrackedEventRepository interface {
	GetReceipt(context.Context, string, string, string) (*TrackedEventResult, string, error)
	Ingest(context.Context, TrackedEventCommand) (*TrackedEventResult, error)
	List(context.Context, string, string, int) ([]TrackedEventResult, error)
}

func BusinessJSONPath(value any, path string) (any, bool) {
	if path == "$" {
		return value, true
	}
	if !strings.HasPrefix(path, "$.") {
		return nil, false
	}
	for _, key := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = obj[key]
		if !ok {
			return nil, false
		}
	}
	return value, true
}
func MapTrackedEvent(raw map[string]any, m *BusinessDefinition) (map[string]any, error) {
	if m == nil {
		return raw, nil
	}
	mapped := map[string]any{}
	fields, ok := m.Settings["fields"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mapping fields must be an object")
	}
	for key, p := range fields {
		path, ok := p.(string)
		if !ok {
			return nil, fmt.Errorf("mapping path for %s must be a string", key)
		}
		if v, exists := BusinessJSONPath(raw, path); exists {
			mapped[key] = v
		}
	}
	if code, ok := m.Settings["event_code"].(string); ok && code != "" {
		mapped["event_code"] = code
	}
	return mapped, nil
}
func NormalizeTrackedEvent(raw map[string]any, d, m *BusinessDefinition, now time.Time) (*TrackedEvent, error) {
	mapped, err := MapTrackedEvent(raw, m)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(mapped)
	if err != nil {
		return nil, err
	}
	var e TrackedEvent
	if err = json.Unmarshal(payload, &e); err != nil {
		return nil, fmt.Errorf("invalid event fields: %w", err)
	}
	e.TrustedTouchpointID = ""
	e.AttributePaths = nil
	e.SensitiveAttributes = nil
	if strings.TrimSpace(e.ReportID) == "" || len(e.ReportID) > 255 {
		return nil, fmt.Errorf("report_id must contain 1 to 255 characters")
	}
	if e.EventCode == "" {
		return nil, fmt.Errorf("event_code is required")
	}
	if e.EventCode != d.Key {
		return nil, fmt.Errorf("event_code does not match definition")
	}
	if e.EventName == "" {
		e.EventName = d.Name
	}
	if m != nil {
		var mappings []AttributeMapping
		rawMappings, _ := json.Marshal(m.Settings["attribute_mappings"])
		if string(rawMappings) != "null" {
			if err = json.Unmarshal(rawMappings, &mappings); err != nil {
				return nil, fmt.Errorf("invalid attribute mappings")
			}
		}
		if e.CustomerAttributes == nil {
			e.CustomerAttributes = map[string]any{}
		}
		input := map[string]any{"data": e.Data, "customer_attributes": e.CustomerAttributes}
		for _, mapping := range mappings {
			if mapping.Action == "lookup" || mapping.Action == "bind" {
				if value, ok := BusinessJSONPath(input, mapping.Path); ok && value != nil {
					e.CustomerAttributes[mapping.Attribute] = value
				}
			}
		}
	}
	if e.CustomerAttributes == nil || len(e.CustomerAttributes) == 0 {
		return nil, fmt.Errorf("customer_attributes must be a nonempty object")
	}
	if e.Data == nil {
		return nil, fmt.Errorf("data must be an object")
	}
	if e.TriggeredAt.IsZero() {
		return nil, fmt.Errorf("triggered_at is required")
	}
	if err = ValidateBusinessValue(d.Schema, e.Data, "data"); err != nil {
		return nil, err
	}
	e.ReportedAt = now.UTC()
	e.DefinitionRevision = d.Revision
	if m != nil {
		e.MappingRevision = m.Revision
	}
	mode, _ := d.Settings["expiry_mode"].(string)
	switch mode {
	case "never":
	case "duration":
		seconds, _ := d.Settings["ttl_seconds"].(float64)
		if n, ok := d.Settings["ttl_seconds"].(int); ok {
			seconds = float64(n)
		}
		if seconds <= 0 || seconds > 315360000 {
			return nil, fmt.Errorf("ttl_seconds must be positive and at most ten years")
		}
		if e.ExpiresAt == nil {
			expiry := e.TriggeredAt.Add(time.Duration(seconds) * time.Second)
			e.ExpiresAt = &expiry
		}
	case "required", "":
		if e.ExpiresAt == nil {
			return nil, fmt.Errorf("expires_at is required by event definition")
		}
	default:
		return nil, fmt.Errorf("invalid expiry_mode")
	}
	if e.ExpiresAt != nil && e.ExpiresAt.Before(e.TriggeredAt) {
		return nil, fmt.Errorf("expires_at must not precede triggered_at")
	}
	e.ProcessingStatus = "accepted"
	if e.Backfill {
		e.ProcessingStatus = "backfill"
	} else if e.ExpiresAt != nil && !e.ExpiresAt.After(now) {
		e.ProcessingStatus = "expired"
	}
	return &e, nil
}
