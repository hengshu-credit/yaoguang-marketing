package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEventSchemaExtensionPreservesExistingValidation(t *testing.T) {
	old := map[string]any{"type": "object", "properties": map[string]any{"order_id": map[string]any{"type": "string"}}, "required": []any{"order_id"}, "additionalProperties": false}
	extra := map[string]any{"type": "object", "properties": map[string]any{"device_fingerprint": map[string]any{"type": "string"}}}
	merged, err := ExtendEventSchema(old, extra)
	require.NoError(t, err)
	require.NoError(t, ValidateBusinessValue(merged, map[string]any{"order_id": "o1"}, "data"))
	require.NoError(t, ValidateBusinessValue(merged, map[string]any{"order_id": "o1", "device_fingerprint": "fp"}, "data"))
	require.Error(t, ValidateBusinessValue(merged, map[string]any{"order_id": "o1", "device_fingerprint": 12}, "data"))
	require.NotContains(t, old["properties"], "device_fingerprint")
	extra["required"] = []string{"device_fingerprint"}
	_, err = ExtendEventSchema(old, extra)
	require.Error(t, err)
	extra = map[string]any{"type": "object", "properties": map[string]any{"order_id": map[string]any{"type": "number"}}}
	_, err = ExtendEventSchema(old, extra)
	require.Error(t, err)
}

func TestStateFieldAcceptsConfiguredDottedAttributeKeys(t *testing.T) {
	value, ok := StateField(map[string]any{"device.fingerprint": "fp", "device": map[string]any{"fingerprint": "nested"}}, "device.fingerprint")
	require.True(t, ok)
	require.Equal(t, "fp", value)
	value, ok = StateField(map[string]any{"device": map[string]any{"fingerprint": "nested"}}, "$.device.fingerprint")
	require.True(t, ok)
	require.Equal(t, "nested", value)
}
