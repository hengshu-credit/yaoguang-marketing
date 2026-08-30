package integration

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryProviderUnknownIntegration(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	reserved := fixture.reserve(t, 2)
	claims, err := fixture.queue.ClaimPending(context.Background(), fixture.workspaceID, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	attempt, err := fixture.repository.StartAttempt(context.Background(), fixture.workspaceID, domain.DeliveryAttemptStart{
		IntentID: reserved.Intent.ID, Provider: "smtp", ClaimToken: claims[0].ClaimToken, LeaseExpiresAt: *claims[0].LeaseExpiresAt,
	})
	require.NoError(t, err)
	require.NoError(t, fixture.repository.RecordAttemptOutcome(context.Background(), fixture.workspaceID, attempt.ID, claims[0].ClaimToken, domain.DeliveryAttemptOutcome{
		Status: domain.DeliveryStatusUnknown, ErrorCategory: "transport", ErrorDetail: "connection reset after request write", OccurredAt: time.Now().UTC(),
	}))
	detail, err := fixture.repository.GetDelivery(context.Background(), fixture.workspaceID, reserved.Intent.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeliveryStatusUnknown, detail.Intent.Status)
	require.Len(t, detail.Reconciliations, 1)
	claims, err = fixture.queue.ClaimPending(context.Background(), fixture.workspaceID, 1, time.Minute)
	require.NoError(t, err)
	assert.Empty(t, claims)
}
