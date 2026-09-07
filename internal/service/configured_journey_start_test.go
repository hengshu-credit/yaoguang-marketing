package service

import (
	"context"
	"encoding/json"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type configuredStartStub struct {
	reason string
	expiry *time.Time
	calls  int
}

func (s *configuredStartStub) ConfiguredJourneyStart(context.Context, string, string, time.Time) (string, *time.Time, error) {
	s.calls++
	return s.reason, s.expiry, nil
}
func TestConfiguredInitialJourneyExpiredDoesNotPlanLiveAction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mocks.NewMockAutomationRepository(ctrl)
	node := "email"
	a := &domain.Automation{ID: "flow", Status: domain.AutomationStatusLive, Nodes: []*domain.AutomationNode{{ID: node, Type: domain.NodeTypeEmail, Config: map[string]any{"template_id": "template"}}}}
	repo.EXPECT().GetByID(gomock.Any(), "w", "flow").Return(a, nil).AnyTimes()
	now := time.Now().UTC()
	expiry := now.Add(-time.Second)
	gate := &configuredStartStub{reason: "origin_event_expired", expiry: &expiry}
	executor := &AutomationExecutor{automationRepo: repo, journeyStartGate: gate}
	claim := domain.ContactAutomationClaim{ContactAutomation: domain.ContactAutomation{ID: "instance", AutomationID: "flow", CurrentNodeID: &node, Status: domain.ContactAutomationStatusActive}, OriginEventID: uuidPtr(uuid.New()), StateVersion: 0}
	commit, err := executor.PlanClaimedJourneyStep(context.Background(), "w", claim, now)
	require.NoError(t, err)
	require.Equal(t, domain.ContactAutomationStatusExited, commit.ContactAutomation.Status)
	require.Equal(t, "origin_event_expired", *commit.ContactAutomation.ExitReason)
	require.Nil(t, commit.Command)
	require.Nil(t, commit.SideEffect)
	// An already-started journey may continue after its trigger expired.
	claim.StateVersion = 1
	commit, err = executor.PlanClaimedJourneyStep(context.Background(), "w", claim, now)
	require.NoError(t, err)
	require.NotNil(t, commit.Command)
	require.Equal(t, 1, gate.calls)
}
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

func TestConfiguredInitialSideEffectExpiredDoesNotExecute(t *testing.T) {
	node := &recordingSideEffectNodeExecutor{}
	executor := &AutomationExecutor{nodeExecutors: map[domain.NodeType]NodeExecutor{domain.NodeTypeWebhook: node}}
	expiry := time.Now().Add(-time.Second)
	body, err := json.Marshal(journeySideEffectData{ContactAutomationID: "instance", AutomationID: "flow", CustomerID: uuid.NewString(), NodeID: "webhook", NodeType: domain.NodeTypeWebhook, StartExpiresAt: &expiry})
	require.NoError(t, err)
	err = executor.ExecuteJourneySideEffect(context.Background(), domain.EventEnvelope{WorkspaceID: "w", Data: body}, "effect")
	require.NoError(t, err)
	require.Empty(t, node.calls)
}
