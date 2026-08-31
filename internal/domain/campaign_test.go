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

func TestCampaignVersionAcceptsExactlyOneRecipientSource(t *testing.T) {
	validList := CampaignVersion{CampaignID: "campaign-1", Version: 1, ListID: "news", Channel: "email",
		Variants: []CampaignVariant{{ID: "default", WeightBP: 10_000}}}
	require.NoError(t, validList.Validate())

	both := validList
	both.AudienceID, both.AudienceVersion = "audience-1", 1
	assert.ErrorContains(t, both.Validate(), "exactly one")

	neither := validList
	neither.ListID = ""
	assert.ErrorContains(t, neither.Validate(), "exactly one")

	incompleteAudience := validList
	incompleteAudience.ListID, incompleteAudience.AudienceID = "", "audience-1"
	assert.ErrorContains(t, incompleteAudience.Validate(), "exactly one")
}

func TestCampaignRunCarriesTheExactResolvedAudienceSource(t *testing.T) {
	run := CampaignRun{
		AudienceID:      "audience-1",
		AudienceVersion: 7,
		AudienceBuildID: "build-1",
	}
	assert.Equal(t, "audience-1", run.AudienceID)
	assert.Equal(t, 7, run.AudienceVersion)
	assert.Equal(t, "build-1", run.AudienceBuildID)
}
