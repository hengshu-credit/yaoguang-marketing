package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

type staticDueJourneySource struct {
	journeys []*domain.ContactAutomationWithWorkspace
	err      error
}

func (s *staticDueJourneySource) GetScheduledContactAutomationsGlobal(context.Context, time.Time, int) ([]*domain.ContactAutomationWithWorkspace, error) {
	return s.journeys, s.err
}

type recordingJourneyPublisher struct{ messages []broker.Message }

func (p *recordingJourneyPublisher) Publish(_ context.Context, message broker.Message) error {
	p.messages = append(p.messages, message)
	return nil
}

func TestRealtimeJourneySchedulerPublishesStableWakeMessages(t *testing.T) {
	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	nodeID := "delay-1"
	source := &staticDueJourneySource{journeys: []*domain.ContactAutomationWithWorkspace{{
		WorkspaceID: "workspace-1",
		ContactAutomation: domain.ContactAutomation{
			ID: "ca-1", AutomationID: "automation-1", ContactEmail: "person@example.com",
			CurrentNodeID: &nodeID, Status: domain.ContactAutomationStatusActive, ScheduledAt: &now,
		},
	}}}
	publisher := &recordingJourneyPublisher{}
	scheduler, err := NewRealtimeJourneyScheduler(source, publisher, time.Second, 100)
	require.NoError(t, err)
	scheduler.now = func() time.Time { return now }

	processed, err := scheduler.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	processed, err = scheduler.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, publisher.messages, 2)
	assert.Equal(t, publisher.messages[0].ID, publisher.messages[1].ID)
	assert.Equal(t, "journey.resume.automation-1", publisher.messages[0].RoutingKey)
}
