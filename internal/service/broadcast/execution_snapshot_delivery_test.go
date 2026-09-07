package broadcast

import (
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutionSnapshotRunSeparatesDeliveryEffects(t *testing.T) {
	campaign := &domain.Broadcast{ID: "campaign"}
	campaign.Audience.CampaignRunID = "run-1"
	first, err := broadcastDeliveryEffectKey("ws", campaign, "customer", "a@example.org", "single", "1", "template")
	require.NoError(t, err)
	campaign.Audience.CampaignRunID = "run-2"
	second, err := broadcastDeliveryEffectKey("ws", campaign, "customer", "a@example.org", "single", "1", "template")
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}
