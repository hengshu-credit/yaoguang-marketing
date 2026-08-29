package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/broker"
)

type recordingJourneyClaims struct {
	claim       *domain.ContactAutomationClaim
	acquired    bool
	claimErr    error
	committed   bool
	commitErr   error
	claimCalls  int
	commitCalls int
	releases    int
	lastCommit  domain.JourneyStateCommit
}

func (r *recordingJourneyClaims) ClaimContactAutomation(context.Context, string, string, string, time.Time, time.Duration) (*domain.ContactAutomationClaim, bool, error) {
	r.claimCalls++
	return r.claim, r.acquired, r.claimErr
}

func (r *recordingJourneyClaims) RenewContactAutomationClaim(context.Context, string, string, uuid.UUID, time.Time, time.Duration) (bool, error) {
	return true, nil
}

func (r *recordingJourneyClaims) CommitContactAutomationState(_ context.Context, _ string, _ domain.ContactAutomationClaim, commit domain.JourneyStateCommit) (bool, error) {
	r.commitCalls++
	r.lastCommit = commit
	return r.committed, r.commitErr
}

func (r *recordingJourneyClaims) ReleaseContactAutomationClaim(context.Context, string, string, uuid.UUID) (bool, error) {
	r.releases++
	return true, nil
}

type recordingJourneyPlanner struct {
	commit domain.JourneyStateCommit
	err    error
	calls  int
}

func (p *recordingJourneyPlanner) PlanClaimedJourneyStep(context.Context, string, domain.ContactAutomationClaim, time.Time) (domain.JourneyStateCommit, error) {
	p.calls++
	return p.commit, p.err
}

func TestJourneyWorkerAcknowledgesWakeWhenClaimIsAlreadyHeld(t *testing.T) {
	repo := &recordingJourneyClaims{}
	planner := &recordingJourneyPlanner{}
	worker, err := NewJourneyWorker(repo, planner, "journey-worker-1", time.Minute)
	require.NoError(t, err)

	decision := worker.HandleDelivery(context.Background(), journeyWakeMessage(t, "ca-1"))
	assert.Equal(t, broker.Ack, decision.Action)
	assert.Equal(t, 1, repo.claimCalls)
	assert.Zero(t, planner.calls)
}

func TestJourneyWorkerCommitsClaimedStepOnce(t *testing.T) {
	claim := journeyTestClaim()
	repo := &recordingJourneyClaims{claim: &claim, acquired: true, committed: true}
	planner := &recordingJourneyPlanner{commit: domain.JourneyStateCommit{ContactAutomation: claim.ContactAutomation}}
	worker, err := NewJourneyWorker(repo, planner, "journey-worker-1", time.Minute)
	require.NoError(t, err)

	decision := worker.HandleDelivery(context.Background(), journeyWakeMessage(t, "ca-1"))
	assert.Equal(t, broker.Ack, decision.Action)
	assert.Equal(t, 1, repo.commitCalls)
	assert.Equal(t, 1, planner.calls)
}

func TestJourneyWorkerReleasesClaimWhenPlanningFails(t *testing.T) {
	claim := journeyTestClaim()
	repo := &recordingJourneyClaims{claim: &claim, acquired: true}
	planner := &recordingJourneyPlanner{err: errors.New("temporary dependency failure")}
	worker, err := NewJourneyWorker(repo, planner, "journey-worker-1", time.Minute)
	require.NoError(t, err)

	decision := worker.HandleDelivery(context.Background(), journeyWakeMessage(t, "ca-1"))
	assert.Equal(t, broker.Retry, decision.Action)
	assert.Equal(t, broker.Retry30Seconds, decision.RetryTier)
	assert.Equal(t, 1, repo.releases)
}

func TestJourneyWorkerDeadLettersMalformedWake(t *testing.T) {
	worker, err := NewJourneyWorker(&recordingJourneyClaims{}, &recordingJourneyPlanner{}, "journey-worker-1", time.Minute)
	require.NoError(t, err)
	decision := worker.HandleDelivery(context.Background(), broker.Message{ID: uuid.New(), Body: []byte(`{"type":`)})
	assert.Equal(t, broker.DeadLetter, decision.Action)
}

func journeyTestClaim() domain.ContactAutomationClaim {
	return domain.ContactAutomationClaim{
		ContactAutomation: domain.ContactAutomation{
			ID: "ca-1", AutomationID: "automation-1", ContactEmail: "person@example.com",
			Status: domain.ContactAutomationStatusActive,
		},
		ClaimToken: uuid.New(), StateVersion: 2, AutomationVersion: 4,
	}
}

func journeyWakeMessage(t *testing.T, contactAutomationID string) broker.Message {
	t.Helper()
	messageID := uuid.New()
	eventID := uuid.New()
	envelope := domain.EventEnvelope{
		ID: messageID, EventID: eventID, Type: "journey.start", SchemaVersion: 1,
		WorkspaceID: "workspace-1",
		Subject:     domain.EventSubject{Type: "contact_automation", ID: contactAutomationID, ContactEmail: "person@example.com"},
		Source:      "rule-worker", OccurredAt: time.Now().UTC(), ReceivedAt: time.Now().UTC(), CorrelationID: eventID,
		Data: json.RawMessage(`{"contact_automation_id":"ca-1"}`),
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	return broker.Message{ID: messageID, Body: body}
}
