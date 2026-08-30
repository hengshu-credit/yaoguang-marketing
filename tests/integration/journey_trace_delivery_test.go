package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/app"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJourneyTraceIncludesFrequencySuppressedDeliveryIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface { return app.NewApp(cfg) })
	defer suite.Cleanup()

	workspace, db, customerID, email := createJourneyTestFixture(t, suite)
	insertLiveJourneyAutomation(t, db, workspace.ID, "journey-trace", "every_time")
	eventID := uuid.New().String()
	outcome, contactAutomationID, _ := enrollJourneyCustomer(t, db, "journey-trace", customerID, email, "every_time", &eventID, `{"enabled":false}`)
	require.Equal(t, "enrolled", outcome)

	var instanceID string
	require.NoError(t, db.QueryRow(`SELECT instance.id::text FROM journey_instances instance
		WHERE instance.contact_automation_id = $1`, contactAutomationID).Scan(&instanceID))
	intentID := uuid.New().String()
	_, err := db.Exec(`INSERT INTO delivery_intents (
		id, effect_key, request_hash, source_type, source_id, source_version,
		customer_id, legacy_identity, channel, template_id, template_version,
		node_or_phase, occurrence, variant, status, suppression_reason, metadata
	) VALUES ($1::uuid, $2, $3, 'automation', 'journey-trace', '1', $4::uuid, $5,
		'email', 'welcome-email', 1, 'message-1', $6, 'control', 'suppressed',
		'frequency_cap:trigger', jsonb_build_object('journey_instance_id', $7::text, 'contact_automation_id', $8::text))`,
		intentID, strings.Repeat("b", 64), strings.Repeat("c", 64), customerID, email, eventID, instanceID, contactAutomationID)
	require.NoError(t, err)

	source, err := service.NewPostgresJourneyTraceSource(suite.ServerManager.GetApp().GetWorkspaceRepository())
	require.NoError(t, err)
	traceService, err := service.NewJourneyTraceService(source, nil)
	require.NoError(t, err)

	instances, total, err := traceService.ListInstances(context.Background(), domain.JourneyInstanceListRequest{
		WorkspaceID: workspace.ID,
		Locator:     domain.JourneyCustomerLocator{CustomerID: customerID},
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, instances, 1)
	assert.Equal(t, domain.ContactAutomationStatusActive, instances[0].Status)
	assert.Equal(t, domain.TriggerFrequencyEveryTime, instances[0].Frequency)

	trace, err := traceService.GetTrace(context.Background(), domain.JourneyTraceRequest{
		WorkspaceID:       workspace.ID,
		JourneyInstanceID: instanceID,
	})
	require.NoError(t, err)
	assert.Equal(t, instanceID, trace.Instance.ID)
	require.NotEmpty(t, trace.Decisions)
	assert.Equal(t, "enrolled", trace.Decisions[0].Decision)
	require.NotEmpty(t, trace.Events)
	assert.Equal(t, "enrolled", trace.Events[0].EventType)
	require.Len(t, trace.Deliveries, 1)
	assert.Equal(t, intentID, trace.Deliveries[0].Intent.ID)
	assert.Equal(t, domain.DeliveryStatusSuppressed, trace.Deliveries[0].Intent.Status)
	assert.Equal(t, "frequency_cap:trigger", trace.Deliveries[0].Intent.SuppressionReason)
	assert.Equal(t, domain.ContactAutomationStatusActive, trace.Instance.Status,
		"message suppression must be visible without terminating the Journey")
}
