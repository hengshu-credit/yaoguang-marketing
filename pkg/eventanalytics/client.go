// Package eventanalytics projects authoritative PostgreSQL events into a
// rebuildable ClickHouse read model.
package eventanalytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

type EventProjectionStore interface {
	InsertBatch(context.Context, []domain.EventEnvelope) error
}

type EventProjection struct {
	WorkspaceID   string
	EventID       uuid.UUID
	EventType     string
	SchemaVersion int
	SubjectType   string
	SubjectID     string
	ContactEmail  string
	Source        string
	CorrelationID uuid.UUID
	CausationID   *uuid.UUID
	OccurredAt    time.Time
	ReceivedAt    time.Time
	ProjectedAt   time.Time
	PayloadJSON   string
	EnvelopeJSON  string
}

func NewEventProjection(envelope domain.EventEnvelope, projectedAt time.Time) (EventProjection, error) {
	if err := envelope.Validate(); err != nil {
		return EventProjection{}, fmt.Errorf("invalid event projection: %w", err)
	}
	if !json.Valid(envelope.Data) {
		return EventProjection{}, fmt.Errorf("event projection payload is invalid json")
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return EventProjection{}, fmt.Errorf("marshal event projection envelope: %w", err)
	}
	return EventProjection{
		WorkspaceID: envelope.WorkspaceID, EventID: envelope.EventID,
		EventType: envelope.Type, SchemaVersion: envelope.SchemaVersion,
		SubjectType: envelope.Subject.Type, SubjectID: envelope.Subject.ID,
		ContactEmail: envelope.Subject.ContactEmail, Source: envelope.Source,
		CorrelationID: envelope.CorrelationID, CausationID: envelope.CausationID,
		OccurredAt: envelope.OccurredAt.UTC(), ReceivedAt: envelope.ReceivedAt.UTC(),
		ProjectedAt: projectedAt.UTC(), PayloadJSON: string(envelope.Data), EnvelopeJSON: string(encoded),
	}, nil
}
