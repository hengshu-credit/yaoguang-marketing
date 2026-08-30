package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignVersionActivatedIsImmutable(t *testing.T) {
	activated := time.Now().UTC()
	version := CampaignVersion{ActivatedAt: &activated}
	assert.ErrorContains(t, version.EnsureMutable(), "immutable")
}

func TestCampaignVariantAssignmentIsDeterministic(t *testing.T) {
	version := CampaignVersion{CampaignID: "campaign-1", Version: 1, AudienceID: "audience-1", AudienceVersion: 1, Channel: "email", Variants: []CampaignVariant{{ID: "a", WeightBP: 5000}, {ID: "b", WeightBP: 5000}}}
	first, err := version.AssignVariant("customer-1", "run-seed")
	require.NoError(t, err)
	second, err := version.AssignVariant("customer-1", "run-seed")
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestCampaignVariantWeightsMustCoverWholePopulation(t *testing.T) {
	version := CampaignVersion{CampaignID: "campaign-1", Version: 1, AudienceID: "audience-1", AudienceVersion: 1, Channel: "email", Variants: []CampaignVariant{{ID: "a", WeightBP: 9999}}}
	assert.ErrorContains(t, version.Validate(), "10000")
}
