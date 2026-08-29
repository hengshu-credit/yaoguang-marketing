package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

type ScheduledJourneySource interface {
	GetScheduledContactAutomationsGlobal(context.Context, time.Time, int) ([]*domain.ContactAutomationWithWorkspace, error)
}

type RealtimeJourneyScheduler struct {
	source    ScheduledJourneySource
	publisher broker.Publisher
	interval  time.Duration
	batchSize int
	now       func() time.Time
}

func NewRealtimeJourneyScheduler(
	source ScheduledJourneySource,
	publisher broker.Publisher,
	interval time.Duration,
	batchSize int,
) (*RealtimeJourneyScheduler, error) {
	if source == nil || publisher == nil {
		return nil, errors.New("journey source and publisher are required")
	}
	if interval <= 0 || batchSize <= 0 {
		return nil, errors.New("journey scheduler interval and batch size must be positive")
	}
	return &RealtimeJourneyScheduler{
		source: source, publisher: publisher, interval: interval, batchSize: batchSize,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *RealtimeJourneyScheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if _, err := s.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			// Keep polling: the confirmed publisher reports broker outages and a
			// due row remains authoritative in PostgreSQL for the next tick.
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *RealtimeJourneyScheduler) ProcessOnce(ctx context.Context) (int, error) {
	now := s.now().UTC()
	journeys, err := s.source.GetScheduledContactAutomationsGlobal(ctx, now, s.batchSize)
	if err != nil {
		return 0, fmt.Errorf("list due realtime journeys: %w", err)
	}
	processed := 0
	var publishErrors []error
	for _, journey := range journeys {
		message, err := scheduledJourneyMessage(journey, now)
		if err != nil {
			publishErrors = append(publishErrors, err)
			continue
		}
		if err := s.publisher.Publish(ctx, message); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("publish journey wake %s: %w", journey.ID, err))
			continue
		}
		processed++
	}
	return processed, errors.Join(publishErrors...)
}

func scheduledJourneyMessage(journey *domain.ContactAutomationWithWorkspace, now time.Time) (broker.Message, error) {
	if journey == nil || journey.WorkspaceID == "" || journey.ID == "" || journey.AutomationID == "" {
		return broker.Message{}, errors.New("scheduled journey identity is incomplete")
	}
	nodeID := ""
	if journey.CurrentNodeID != nil {
		nodeID = *journey.CurrentNodeID
	}
	scheduledAt := time.Time{}
	if journey.ScheduledAt != nil {
		scheduledAt = journey.ScheduledAt.UTC()
	}
	identity := fmt.Sprintf("%s:%s:%s:%s", journey.WorkspaceID, journey.ID, nodeID, scheduledAt.Format(time.RFC3339Nano))
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity+":event"))
	messageID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity+":message"))
	data, _ := json.Marshal(map[string]interface{}{
		"contact_automation_id": journey.ID,
		"automation_id":         journey.AutomationID,
		"scheduled_at":          scheduledAt,
	})
	envelope := domain.EventEnvelope{
		ID: messageID, EventID: eventID, Type: "journey.resume", SchemaVersion: 1,
		WorkspaceID: journey.WorkspaceID,
		Subject: domain.EventSubject{
			Type: "contact_automation", ID: journey.ID, ContactEmail: journey.ContactEmail,
		},
		Source: "journey-scheduler", OccurredAt: now, ReceivedAt: now,
		CorrelationID: eventID, Data: data,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return broker.Message{}, fmt.Errorf("marshal journey wake: %w", err)
	}
	return broker.Message{
		ID: messageID, CorrelationID: eventID, Exchange: broker.JobsExchange,
		RoutingKey: "journey.resume." + journey.AutomationID,
		Type:       envelope.Type, SchemaVersion: 1, Timestamp: now, Body: body,
	}, nil
}
