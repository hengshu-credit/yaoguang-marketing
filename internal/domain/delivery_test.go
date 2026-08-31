package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryEffectKeyIsStableAfterCanonicalNormalization(t *testing.T) {
	base := DeliveryEffectKeyInput{
		WorkspaceID:   "workspace-1",
		SourceType:    "automation",
		SourceID:      "automation-42",
		SourceVersion: "7",
		CustomerID:    "customer-9",
		NodeOrPhase:   "send-email",
		Occurrence:    "event-20260830T123000Z",
		Variant:       "A",
	}

	canonical, err := base.EffectKey()
	require.NoError(t, err)

	equivalent := base
	equivalent.WorkspaceID = "  workspace-1  "
	equivalent.SourceType = " AUTOMATION "
	equivalent.NodeOrPhase = " send-email "
	equivalent.Variant = "Ａ" // NFKC-normalizes to ASCII A.
	normalized, err := equivalent.EffectKey()
	require.NoError(t, err)

	assert.Equal(t, canonical, normalized)
	assert.Len(t, canonical, 64)
}

func TestDeliveryEffectKeyChangesForEveryBusinessDimension(t *testing.T) {
	base := DeliveryEffectKeyInput{
		WorkspaceID: "workspace-1", SourceType: "campaign", SourceID: "campaign-1",
		SourceVersion: "1", CustomerID: "customer-1", NodeOrPhase: "send",
		Occurrence: "run-1", Variant: "control",
	}
	baseKey, err := base.EffectKey()
	require.NoError(t, err)

	cases := map[string]func(*DeliveryEffectKeyInput){
		"workspace":      func(v *DeliveryEffectKeyInput) { v.WorkspaceID = "workspace-2" },
		"source type":    func(v *DeliveryEffectKeyInput) { v.SourceType = "automation" },
		"source id":      func(v *DeliveryEffectKeyInput) { v.SourceID = "campaign-2" },
		"source version": func(v *DeliveryEffectKeyInput) { v.SourceVersion = "2" },
		"customer":       func(v *DeliveryEffectKeyInput) { v.CustomerID = "customer-2" },
		"node":           func(v *DeliveryEffectKeyInput) { v.NodeOrPhase = "follow-up" },
		"occurrence":     func(v *DeliveryEffectKeyInput) { v.Occurrence = "run-2" },
		"variant":        func(v *DeliveryEffectKeyInput) { v.Variant = "experiment" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			key, keyErr := candidate.EffectKey()
			require.NoError(t, keyErr)
			assert.NotEqual(t, baseKey, key)
		})
	}
}

func TestDeliveryEffectKeyRejectsMissingRequiredDimensions(t *testing.T) {
	valid := DeliveryEffectKeyInput{
		WorkspaceID: "workspace-1", SourceType: "campaign", SourceID: "campaign-1",
		SourceVersion: "1", CustomerID: "customer-1", NodeOrPhase: "send",
		Occurrence: "run-1", Variant: "control",
	}

	cases := map[string]func(*DeliveryEffectKeyInput){
		"workspace_id":   func(v *DeliveryEffectKeyInput) { v.WorkspaceID = "" },
		"source_type":    func(v *DeliveryEffectKeyInput) { v.SourceType = "" },
		"source_id":      func(v *DeliveryEffectKeyInput) { v.SourceID = "" },
		"source_version": func(v *DeliveryEffectKeyInput) { v.SourceVersion = "" },
		"customer_id":    func(v *DeliveryEffectKeyInput) { v.CustomerID = "" },
		"node_or_phase":  func(v *DeliveryEffectKeyInput) { v.NodeOrPhase = "" },
		"occurrence":     func(v *DeliveryEffectKeyInput) { v.Occurrence = "" },
		"variant":        func(v *DeliveryEffectKeyInput) { v.Variant = "" },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			_, err := candidate.EffectKey()
			assert.ErrorContains(t, err, field)
		})
	}
}

func TestDeliveryStatusTransitionPolicy(t *testing.T) {
	assert.True(t, DeliveryStatusPlanned.CanTransitionTo(DeliveryStatusReserved))
	assert.True(t, DeliveryStatusReserved.CanTransitionTo(DeliveryStatusQueued))
	assert.True(t, DeliveryStatusQueued.CanTransitionTo(DeliveryStatusSubmitting))
	assert.True(t, DeliveryStatusQueued.CanTransitionTo(DeliveryStatusSuppressed))
	assert.True(t, DeliveryStatusSubmitting.CanTransitionTo(DeliveryStatusProviderAccepted))
	assert.True(t, DeliveryStatusProviderAccepted.CanTransitionTo(DeliveryStatusConfirmed))
	assert.True(t, DeliveryStatusTransientFailed.CanTransitionTo(DeliveryStatusQueued))
	assert.True(t, DeliveryStatusUnknown.CanTransitionTo(DeliveryStatusConfirmed))

	assert.False(t, DeliveryStatusConfirmed.CanTransitionTo(DeliveryStatusQueued))
	assert.False(t, DeliveryStatusSuppressed.CanTransitionTo(DeliveryStatusSubmitting))
	assert.False(t, DeliveryStatusCancelled.CanTransitionTo(DeliveryStatusQueued))
	assert.False(t, DeliveryStatus("invalid").Valid())
	assert.True(t, DeliveryStatusDeferred.Valid())
}
