package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventReportingConfigurationValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings map[string]any
		status   string
		field    string
	}{
		{"legacy expiration stays required", map[string]any{}, "active", ""},
		{"browser and trusted backend conflict", map[string]any{"browser_enabled": true, "authoritative": true}, "draft", "settings.browser_enabled"},
		{"invalid legacy event can be archived", map[string]any{"browser_enabled": true, "authoritative": true}, "archived", ""},
		{"incomplete browser draft", map[string]any{"browser_enabled": true}, "draft", ""},
		{"origin required on activation", map[string]any{"browser_enabled": true}, "active", "settings.allowed_browser_origins"},
		{"valid origin", map[string]any{"browser_enabled": true, "allowed_browser_origins": []any{"https://app.example.com"}}, "active", ""},
		{"path is not origin", map[string]any{"browser_enabled": true, "allowed_browser_origins": []any{"https://app.example.com/page"}}, "active", "settings.allowed_browser_origins"},
		{"empty restriction is not unrestricted", map[string]any{"allowed_sources": []any{}}, "active", "settings.allowed_sources"},
		{"valid restriction", map[string]any{"allowed_sources": []any{"erp"}}, "active", ""},
		{"duration draft incomplete", map[string]any{"expiry_mode": "duration"}, "draft", ""},
		{"duration needs value", map[string]any{"expiry_mode": "duration"}, "active", "settings.ttl_seconds"},
		{"fractional seconds rejected", map[string]any{"expiry_mode": "duration", "ttl_seconds": 1.5}, "active", "settings.ttl_seconds"},
		{"large duration rejected", map[string]any{"expiry_mode": "duration", "ttl_seconds": 315360001}, "active", "settings.ttl_seconds"},
		{"valid seconds", map[string]any{"expiry_mode": "duration", "ttl_seconds": 90}, "active", ""},
		{"object needs id path", map[string]any{"subject_type": "order"}, "active", "settings.subject_id_path"},
		{"unsupported array path", map[string]any{"subject_type": "order", "subject_id_path": "$.data.orders[0].id"}, "active", "settings.subject_id_path"},
		{"cleared attributes path must be omitted", map[string]any{"subject_type": "order", "subject_id_path": "$.data.id", "subject_attributes_path": ""}, "active", "settings.subject_attributes_path"},
		{"browser object mismatch", map[string]any{"subject_type": "order", "subject_id_path": "$.data.id", "browser_enabled": true}, "active", "settings.subject_type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := BusinessDefinition{Kind: "event", Key: "order.created", Name: "Order created", Schema: map[string]any{"type": "object"}, Settings: tc.settings, Status: tc.status}
			err := d.Validate()
			if tc.field == "" {
				require.NoError(t, err)
				return
			}
			var configError *BusinessConfigurationError
			require.True(t, errors.As(err, &configError), "%v", err)
			require.Equal(t, tc.field, configError.Field)
			var validation ValidationError
			require.ErrorAs(t, err, &validation)
		})
	}
}

func TestEventMappingClearedOptionalPathAndActivation(t *testing.T) {
	d := BusinessDefinition{Kind: "event_mapping", Key: "erp", Name: "ERP", Status: "draft", Settings: map[string]any{"fields": map[string]any{}}}
	require.NoError(t, d.Validate())
	d.Status = "active"
	require.ErrorContains(t, d.Validate(), "settings.fields.report_id")
	d.Settings["fields"] = map[string]any{"report_id": "$.id", "event_code": "$.name", "data": "$.payload", "customer_attributes": "$.customer", "triggered_at": "$.timestamp", "expires_at": ""}
	require.ErrorContains(t, d.Validate(), "settings.fields.expires_at")
	delete(d.Settings["fields"].(map[string]any), "expires_at")
	require.NoError(t, d.Validate())
	delete(d.Settings["fields"].(map[string]any), "customer_attributes")
	require.ErrorContains(t, d.Validate(), "settings.fields.customer_attributes")
	d.Settings["attribute_mappings"] = []any{map[string]any{"attribute": "customer_id", "path": "$.data.customer_id", "action": "lookup"}}
	require.NoError(t, d.Validate())
}

func TestExactBrowserOriginsAndLiteralPaths(t *testing.T) {
	for _, origin := range []string{"https://app.example.com", "http://localhost:5173", "http://[::1]:8080"} {
		require.True(t, IsExactBrowserOrigin(origin), origin)
	}
	for _, origin := range []string{"https://app.example.com/", "https://user:password@app.example.com", "https://*.example.com", "https://app.example.com?", "https://APP.example.com", "https://app.example.com:443", "https://app.example.com#"} {
		require.False(t, IsExactBrowserOrigin(origin), origin)
	}
	for _, path := range []string{"$", "$.data.order_id", "$.data.客户编号", "$.data.order-id"} {
		require.True(t, IsBusinessObjectPath(path), path)
	}
	for _, path := range []string{"", "$.data..id", "$.data[0]", "$.data.*", "$.constructor.id", "$.data.__proto__"} {
		require.False(t, IsBusinessObjectPath(path), path)
	}
}
