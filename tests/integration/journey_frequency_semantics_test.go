package integration

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/realtimecache"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type denyingMultiFrequencyStore struct {
	deniedPolicyID string
	windows        []realtimecache.WindowReservation
}

func (s *denyingMultiFrequencyStore) ReserveSlidingWindow(
	context.Context, string, string, string, string, string, time.Time, time.Duration, int,
) (realtimecache.WindowResult, error) {
	return realtimecache.WindowResult{Allowed: false}, nil
}

func (s *denyingMultiFrequencyStore) ReserveWindows(
	_ context.Context, _, _, _, _ string, _ time.Time, windows []realtimecache.WindowReservation,
) (realtimecache.MultiWindowResult, error) {
	s.windows = append([]realtimecache.WindowReservation(nil), windows...)
	return realtimecache.MultiWindowResult{
		Allowed:        false,
		DeniedPolicyID: s.deniedPolicyID,
		RetryAfter:     time.Hour,
	}, nil
}

func createJourneyTestFixture(t *testing.T, suite *testutil.IntegrationTestSuite) (*domain.Workspace, *sql.DB, string, string) {
	t.Helper()
	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	workspace, err := suite.DataFactory.CreateWorkspace(testutil.WithWorkspaceName("Journey semantics"))
	require.NoError(t, err)
	require.NoError(t, suite.DataFactory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, suite.APIClient.Login(user.Email, "password"))
	db, err := suite.DataFactory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	email := "journey-semantics@example.com"
	response := postCustomer(t, suite.APIClient, "/api/customers.upsert", map[string]interface{}{
		"workspace_id":    workspace.ID,
		"idempotency_key": "journey-semantics-customer",
		"customer": map[string]interface{}{
			"external_user_id": "journey-semantics-1",
			"profile":          map[string]interface{}{"language": "zh-CN", "timezone": "Asia/Shanghai"},
			"identities":       []map[string]interface{}{{"type": "email", "value": email, "primary": true}},
		},
	}, http.StatusOK)
	var envelope customerMutationEnvelope
	decodeCustomerResponse(t, response, &envelope)
	return workspace, db, envelope.Customer.CustomerID, email
}

func insertLiveJourneyAutomation(t *testing.T, db *sql.DB, workspaceID, automationID, frequency string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO automations (
		id, workspace_id, name, status, trigger_config, root_node_id, nodes, stats, version
	) VALUES ($1, $2, $3, 'live', jsonb_build_object('event_kind', 'customer.updated', 'frequency', $4::text),
		'root', '[{"id":"root","type":"trigger"}]'::jsonb, '{}'::jsonb, 1)`,
		automationID, workspaceID, "Journey "+automationID, frequency)
	require.NoError(t, err)
}

func enrollJourneyCustomer(
	t *testing.T, db *sql.DB, automationID, customerID, email, frequency string, eventID *string, guard string,
) (string, string, *time.Time) {
	t.Helper()
	var outcome, contactAutomationID string
	var retryAt *time.Time
	var eventValue interface{}
	if eventID != nil {
		eventValue = *eventID
	}
	err := db.QueryRow(`SELECT outcome, COALESCE(contact_automation_id, ''), retry_at
		FROM automation_enroll_customer($1, $2::uuid, $3, 'root', $4, $5::uuid, $6::jsonb, 1, 'customer_event')`,
		automationID, customerID, email, frequency, eventValue, guard).Scan(&outcome, &contactAutomationID, &retryAt)
	require.NoError(t, err)
	return outcome, contactAutomationID, retryAt
}

func TestJourneyFrequencySemanticsIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface { return app.NewApp(cfg) })
	defer suite.Cleanup()

	workspace, db, customerID, email := createJourneyTestFixture(t, suite)
	insertLiveJourneyAutomation(t, db, workspace.ID, "journey-once", "once")
	insertLiveJourneyAutomation(t, db, workspace.ID, "journey-every-time", "every_time")
	insertLiveJourneyAutomation(t, db, workspace.ID, "journey-guarded", "every_time")

	eventA, eventB, eventC := uuid.New().String(), uuid.New().String(), uuid.New().String()

	t.Run("once creates exactly one instance for a customer", func(t *testing.T) {
		outcome, _, _ := enrollJourneyCustomer(t, db, "journey-once", customerID, email, "once", &eventA, `{"enabled":false}`)
		assert.Equal(t, "enrolled", outcome)
		outcome, _, _ = enrollJourneyCustomer(t, db, "journey-once", customerID, email, "once", &eventB, `{"enabled":false}`)
		assert.Equal(t, "already_once", outcome)

		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM journey_instances instance
			JOIN journey_enrollments enrollment ON enrollment.id = instance.enrollment_id
			WHERE enrollment.automation_id = 'journey-once' AND enrollment.customer_id = $1::uuid`, customerID).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("every time deduplicates a replay but accepts a new event", func(t *testing.T) {
		outcome, _, _ := enrollJourneyCustomer(t, db, "journey-every-time", customerID, email, "every_time", &eventA, `{"enabled":false}`)
		assert.Equal(t, "enrolled", outcome)
		outcome, _, _ = enrollJourneyCustomer(t, db, "journey-every-time", customerID, email, "every_time", &eventA, `{"enabled":false}`)
		assert.Equal(t, "replayed_event", outcome)
		outcome, _, _ = enrollJourneyCustomer(t, db, "journey-every-time", customerID, email, "every_time", &eventB, `{"enabled":false}`)
		assert.Equal(t, "enrolled", outcome)

		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM journey_instances instance
			JOIN journey_enrollments enrollment ON enrollment.id = instance.enrollment_id
			WHERE enrollment.automation_id = 'journey-every-time' AND enrollment.customer_id = $1::uuid`, customerID).Scan(&count))
		assert.Equal(t, 2, count)
	})

	t.Run("entry guard is independent from message frequency control", func(t *testing.T) {
		guard := `{"enabled":true,"max_concurrent":1,"cooldown":3600000000000}`
		outcome, _, _ := enrollJourneyCustomer(t, db, "journey-guarded", customerID, email, "every_time", &eventA, guard)
		assert.Equal(t, "enrolled", outcome)
		outcome, _, retryAt := enrollJourneyCustomer(t, db, "journey-guarded", customerID, email, "every_time", &eventC, guard)
		assert.Equal(t, "guard_deferred", outcome)
		assert.NotNil(t, retryAt)

		policyRepo := repository.NewFrequencyPolicyRepositoryWithDB(db)
		campaignID, triggerID, globalID := uuid.New().String(), uuid.New().String(), uuid.New().String()
		for _, policy := range []domain.FrequencyPolicy{
			{ID: campaignID, Version: 1, Name: "Campaign hourly", Scope: domain.FrequencyScopeCampaign, ScopeRef: "campaign-1", Channel: "email", MaxEvents: 1, WindowKind: domain.FrequencyWindowSliding, WindowSeconds: 3600, DenyAction: domain.FrequencyActionSuppress, Priority: 300, Enabled: true, CreatedAt: time.Now().UTC()},
			{ID: triggerID, Version: 1, Name: "Trigger daily", Scope: domain.FrequencyScopeTrigger, ScopeRef: "journey-every-time", Channel: "email", MaxEvents: 2, WindowKind: domain.FrequencyWindowSliding, WindowSeconds: 86400, DenyAction: domain.FrequencyActionSuppress, Priority: 200, Enabled: true, CreatedAt: time.Now().UTC()},
			{ID: globalID, Version: 1, Name: "Workspace weekly", Scope: domain.FrequencyScopeWorkspaceGlobal, Channel: "email", MaxEvents: 5, WindowKind: domain.FrequencyWindowSliding, WindowSeconds: 604800, DenyAction: domain.FrequencyActionSuppress, Priority: 100, Enabled: true, CreatedAt: time.Now().UTC()},
		} {
			require.NoError(t, policyRepo.SaveFrequencyPolicy(context.Background(), workspace.ID, policy))
		}

		store := &denyingMultiFrequencyStore{deniedPolicyID: triggerID + ":v1"}
		limiter, err := service.NewFrequencyLimiter(store)
		require.NoError(t, err)
		frequencyService, err := service.NewFrequencyPolicyService(policyRepo, limiter)
		require.NoError(t, err)
		decision, err := frequencyService.Evaluate(context.Background(), domain.FrequencyEvaluationRequest{
			WorkspaceID: workspace.ID,
			CustomerID:  customerID,
			Channel:     "email",
			EffectKey:   strings.Repeat("a", 64),
			CampaignRef: "campaign-1",
			TriggerRef:  "journey-every-time",
			OccurredAt:  time.Now().UTC(),
		})
		require.NoError(t, err)
		assert.False(t, decision.Allowed)
		assert.Equal(t, domain.FrequencyScopeTrigger, decision.MatchedScope)
		require.Len(t, store.windows, 3, "campaign, trigger and workspace-global caps must be evaluated atomically")

		var journeyCount int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM journey_instances instance
			JOIN journey_enrollments enrollment ON enrollment.id = instance.enrollment_id
			WHERE enrollment.automation_id = 'journey-every-time' AND enrollment.customer_id = $1::uuid`, customerID).Scan(&journeyCount))
		assert.Equal(t, 2, journeyCount, "a denied message must not remove or block Journey enrollment")
	})
}
