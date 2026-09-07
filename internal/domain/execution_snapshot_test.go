package domain

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutionSnapshotFreezesConfigurationAndTemplate(t *testing.T) {
	config := map[string]any{"filters": map[string]any{"product": "A"}, "api_key": "secret"}
	template := &Template{ID: "T-1", Email: &EmailTemplate{Subject: "Original"}}
	snapshot, err := NewExecutionSnapshot("campaign", "run-1", config, []*Template{template})
	require.NoError(t, err)
	config["filters"].(map[string]any)["product"] = "B"
	template.Email.Subject = "Changed"
	require.Contains(t, string(snapshot.Configuration), `"product":"A"`)
	require.NotContains(t, string(snapshot.Configuration), "secret")
	require.Contains(t, string(snapshot.Materials), "Original")
	require.NotContains(t, string(snapshot.Materials), "Changed")
	require.Len(t, snapshot.ContentHash, 64)
}

func TestExecutionSnapshotHashCanonicalAndSecretRedaction(t *testing.T) {
	first, err := NewExecutionSnapshot("campaign", "run-a", json.RawMessage(`{"b":2,"a":1,"headers":{"Authorization":"Bearer hidden"}}`), nil)
	require.NoError(t, err)
	second, err := NewExecutionSnapshot("campaign", "run-b", map[string]any{"a": 1, "b": 2, "headers": map[string]any{"Authorization": "Bearer other"}}, nil)
	require.NoError(t, err)
	require.Equal(t, first.ContentHash, second.ContentHash)
	require.NotContains(t, string(first.Configuration), "hidden")
}

func TestExecutionEvidenceValidatesActualContent(t *testing.T) {
	e := ExecutionEvidence{ExecutionID: "run-1", Key: "delivery-1:rendered", Kind: "delivery_content", Status: "rendered", Payload: json.RawMessage(`{"body":"Hello Alice"}`)}
	require.NoError(t, e.Validate())
	e.Payload = nil
	require.Error(t, e.Validate())
}
