//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScheduledBroadcast_DirectScheduler_NoHTTPSelfCall is the Cloud counterpart of
// broadcast_scheduled_live_scheduler_test.go. It runs the real TaskScheduler ticker with
// the internal scheduler ENABLED (→ directExecution=true, wired in app.go from
// TaskScheduler.Enabled) and an intentionally UNREACHABLE APIEndpoint ("http://127.0.0.1:1").
//
// This reproduces the production single-pod-per-tenant topology where the app cannot reach
// its own public ingress: dispatching task execution over HTTP would fail with
// connection refused, leaving send_broadcast stuck pending forever (the citymousetours bug).
//
// With direct execution, ExecutePendingTasks runs the task in-process and never touches the
// dead endpoint, so the past-due broadcast completes end-to-end (orchestrator → email queue
// worker → SMTP → Mailpit). If the execution-mode coupling ever regressed back to HTTP
// dispatch, the unreachable APIEndpoint would stall the task and this test would fail.
func TestScheduledBroadcast_DirectScheduler_NoHTTPSelfCall(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuiteWithDirectScheduler(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	uniqueTag := uuid.New().String()[:8]
	uniqueSubject := fmt.Sprintf("Direct-Scheduler Repro %s", uniqueTag)

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Direct Scheduler Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:   "localhost",
				Port:   1025,
				UseTLS: false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Direct Scheduler List"))
	require.NoError(t, err)

	contactEmail := fmt.Sprintf("direct-sched-%s@example.com", uniqueTag)
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Direct Scheduler Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Direct Scheduler Past-Due Broadcast"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	broadcast.TestSettings.Variations[0].TemplateID = template.ID
	updateResp, err := client.UpdateBroadcast(map[string]interface{}{
		"workspace_id":  workspace.ID,
		"id":            broadcast.ID,
		"name":          broadcast.Name,
		"audience":      broadcast.Audience,
		"schedule":      broadcast.Schedule,
		"test_settings": broadcast.TestSettings,
	})
	require.NoError(t, err)
	updateResp.Body.Close()

	// Schedule for 2 minutes ago UTC — past-due, so next_run_after is immediately in the past.
	pastTime := time.Now().UTC().Add(-2 * time.Minute)
	t.Logf("Scheduling broadcast for past time: %s (direct scheduler will pick it up on next tick)",
		pastTime.Format(time.RFC3339))

	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id":           workspace.ID,
		"id":                     broadcast.ID,
		"send_now":               false,
		"scheduled_date":         pastTime.Format("2006-01-02"),
		"scheduled_time":         pastTime.Format("15:04"),
		"timezone":               "UTC",
		"use_recipient_timezone": false,
	})
	require.NoError(t, err)
	defer scheduleResp.Body.Close()
	require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

	// Give the broadcast-scheduled event handler time to create the task row.
	time.Sleep(2 * time.Second)

	taskID, nextRunAfter := findBroadcastTask(t, client, workspace.ID, broadcast.ID)
	require.NotEmpty(t, taskID, "send_broadcast task should have been created")
	require.False(t, nextRunAfter.IsZero(), "task.next_run_after should be set")
	assert.True(t, nextRunAfter.Before(time.Now().UTC()),
		"task.next_run_after (%s) should be in the past for a past-due schedule", nextRunAfter)

	// We do NOT call /api/cron. The live TaskScheduler (500ms ticks) must pick up the task
	// and execute it IN-PROCESS — the APIEndpoint is unreachable, so any HTTP dispatch would
	// stall here.
	deadline := time.Now().Add(15 * time.Second)
	var broadcastStatus, taskStatus string
	var progress float64
	var enqueued int

	for time.Now().Before(deadline) {
		broadcastStatus, enqueued = getBroadcastStatusAndEnqueued(t, client, broadcast.ID)
		taskStatus, progress = getTaskStatusAndProgress(t, client, workspace.ID, taskID)
		t.Logf("poll: broadcast=%s enqueued=%d | task=%s progress=%.1f",
			broadcastStatus, enqueued, taskStatus, progress)

		if broadcastStatus == string(domain.BroadcastStatusProcessed) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Symptom-match failure: a stuck task with the dead endpoint means the coupling regressed
	// to HTTP self-dispatch (the production citymousetours failure).
	if broadcastStatus == string(domain.BroadcastStatusScheduled) &&
		enqueued == 0 &&
		taskStatus == string(domain.TaskStatusPending) &&
		progress == 0 {
		_, _, sendState := getTaskStateDetail(t, client, workspace.ID, taskID)
		t.Logf("send_broadcast state at failure: %+v", sendState)
		t.Fatalf("REGRESSION: scheduler-enabled instance did NOT execute in-process — "+
			"broadcast.status=%s enqueued=%d task.status=%s progress=%.1f (likely fell back to HTTP "+
			"dispatch against the unreachable APIEndpoint)",
			broadcastStatus, enqueued, taskStatus, progress)
	}

	assert.Equal(t, string(domain.BroadcastStatusProcessed), broadcastStatus,
		"past-due scheduled broadcast should reach processed via direct (in-process) execution")
	assert.Equal(t, string(domain.TaskStatusCompleted), taskStatus,
		"send_broadcast task should complete")
	assert.InDelta(t, 100, progress, 0.01, "task progress should reach 100")

	// End-to-end: the email actually landed in Mailpit, proving the full in-process chain
	// (orchestrator → queue worker → SMTP) ran without any HTTP self-call.
	require.NoError(t,
		testutil.WaitForMailpitMessages(t, uniqueSubject, 1, 15*time.Second),
		"broadcast email should arrive in Mailpit")
}
