package domain

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventEnvelopeRoundTripKeepsIdentity(t *testing.T) {
	original := EventEnvelope{
		ID:            uuid.MustParse("018f0000-0000-7000-8000-000000000001"),
		EventID:       uuid.MustParse("018f0000-0000-7000-8000-000000000002"),
		Type:          "contact.updated",
		SchemaVersion: 1,
	}

	body, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded EventEnvelope
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.EventID, decoded.EventID)
}

func TestEventEnvelopeValidateRejectsMissingIdentity(t *testing.T) {
	envelope := EventEnvelope{Type: "contact.updated", SchemaVersion: 1}

	require.ErrorContains(t, envelope.Validate(), "id")
}

func TestCanonicalJSONHashIgnoresObjectFieldOrder(t *testing.T) {
	first, err := CanonicalJSONHash(json.RawMessage(`{"name":"Ada","scores":[1,2],"active":true}`))
	require.NoError(t, err)
	second, err := CanonicalJSONHash(json.RawMessage(`{"active":true,"scores":[1,2],"name":"Ada"}`))
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Len(t, first, 64)
}

func TestCanonicalJSONHashRejectsTrailingDocument(t *testing.T) {
	_, err := CanonicalJSONHash(json.RawMessage(`{"ok":true} {"unexpected":true}`))

	require.Error(t, err)
}

func TestBuildSideEffectKeyIsStable(t *testing.T) {
	first := BuildSideEffectKey("ws", "ca", 2, "node", 7, "email")
	second := BuildSideEffectKey("ws", "ca", 2, "node", 7, "email")

	assert.Equal(t, first, second)
	assert.Len(t, first, 64)
}

func TestBuildSideEffectKeySeparatesEveryDimension(t *testing.T) {
	baseline := BuildSideEffectKey("ws", "ca", 2, "node", 7, "email")
	variants := []string{
		BuildSideEffectKey("other", "ca", 2, "node", 7, "email"),
		BuildSideEffectKey("ws", "other", 2, "node", 7, "email"),
		BuildSideEffectKey("ws", "ca", 3, "node", 7, "email"),
		BuildSideEffectKey("ws", "ca", 2, "other", 7, "email"),
		BuildSideEffectKey("ws", "ca", 2, "node", 8, "email"),
		BuildSideEffectKey("ws", "ca", 2, "node", 7, "webhook"),
	}

	for _, variant := range variants {
		assert.NotEqual(t, baseline, variant)
	}
}
