package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deliveryIntegrationFixture struct {
	suite       *testutil.IntegrationTestSuite
	workspaceID string
	repository  *repository.DeliveryPostgresRepository
	queue       domain.EmailQueueRepository
}

func newDeliveryIntegrationFixture(t *testing.T) deliveryIntegrationFixture {
	t.Helper()
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface { return app.NewApp(cfg) })
	t.Cleanup(func() { suite.Cleanup(); testutil.CleanupTestEnvironment() })
	workspace, err := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("Delivery consistency"))
	require.NoError(t, err)
	db, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	return deliveryIntegrationFixture{
		suite: suite, workspaceID: workspace.ID,
		repository: repository.NewDeliveryRepositoryWithDB(db), queue: repository.NewEmailQueueRepositoryWithDB(db),
	}
}

func (fixture deliveryIntegrationFixture) reserve(t *testing.T, ordinal int) domain.ReserveDeliveryResult {
	t.Helper()
	effectKey, err := (domain.DeliveryEffectKeyInput{
		WorkspaceID: fixture.workspaceID, SourceType: "broadcast", SourceID: "broadcast-crash",
		SourceVersion: "1", CustomerID: fmt.Sprintf("legacy-customer-%d", ordinal),
		NodeOrPhase: "single", Occurrence: fmt.Sprint(ordinal), Variant: "template-1",
	}).EffectKey()
	require.NoError(t, err)
	intent := domain.DeliveryIntent{
		EffectKey: effectKey, RequestHash: fmt.Sprintf("%064x", ordinal+1),
		SourceType: domain.DeliverySourceBroadcast, SourceID: "broadcast-crash", SourceVersion: "1",
		LegacyIdentity: fmt.Sprintf("recipient-%d@example.com", ordinal), Channel: "email",
		NodeOrPhase: "single", Occurrence: fmt.Sprint(ordinal), Variant: "template-1",
		Status: domain.DeliveryStatusReserved,
	}
	entry := &domain.EmailQueueEntry{
		ID: fmt.Sprintf("queue-%d", ordinal), SourceType: domain.EmailQueueSourceBroadcast,
		SourceID: "broadcast-crash", IntegrationID: "integration-1", ProviderKind: domain.EmailProviderKindSMTP,
		ContactEmail: intent.LegacyIdentity, MessageID: fmt.Sprintf("message-%d", ordinal), TemplateID: "template-1",
		Payload: domain.EmailQueuePayload{FromAddress: "sender@example.com", FromName: "Sender", Subject: "Hello", HTMLContent: "<p>Hello</p>"},
	}
	result, err := fixture.repository.ReserveAndEnqueue(context.Background(), fixture.workspaceID, intent, entry)
	require.NoError(t, err)
	return result
}

func TestDeliveryCrashRecoveryIntegration(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	first := fixture.reserve(t, 1)
	replay := fixture.reserve(t, 1)
	assert.True(t, first.Created)
	assert.False(t, replay.Created)
	assert.Equal(t, first.Intent.ID, replay.Intent.ID)

	db, err := fixture.suite.DBManager.GetWorkspaceDB(fixture.workspaceID)
	require.NoError(t, err)
	var intents, queueRows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM delivery_intents WHERE effect_key = $1`, first.Intent.EffectKey).Scan(&intents))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM email_queue WHERE delivery_intent_id = $1`, first.Intent.ID).Scan(&queueRows))
	assert.Equal(t, 1, intents)
	assert.Equal(t, 1, queueRows)

	claims, err := fixture.queue.ClaimPending(context.Background(), fixture.workspaceID, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	attempt, err := fixture.repository.StartAttempt(context.Background(), fixture.workspaceID, domain.DeliveryAttemptStart{
		IntentID: first.Intent.ID, Provider: "smtp", ClaimToken: claims[0].ClaimToken, LeaseExpiresAt: *claims[0].LeaseExpiresAt,
	})
	require.NoError(t, err)
	require.NoError(t, fixture.repository.RecordAttemptOutcome(context.Background(), fixture.workspaceID, attempt.ID, claims[0].ClaimToken, domain.DeliveryAttemptOutcome{
		Status: domain.DeliveryStatusProviderAccepted, OccurredAt: time.Now().UTC(),
	}))
	claims, err = fixture.queue.ClaimPending(context.Background(), fixture.workspaceID, 10, time.Minute)
	require.NoError(t, err)
	assert.Empty(t, claims, "provider_accepted must never be blindly reclaimed after a crash")
}
