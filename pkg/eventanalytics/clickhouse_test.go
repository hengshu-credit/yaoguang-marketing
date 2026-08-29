package eventanalytics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestEventProjectionPreservesTenantSubjectAndTimestamps(t *testing.T) {
	eventID := uuid.New()
	correlationID := uuid.New()
	occurredAt := time.Date(2026, 8, 29, 10, 0, 0, 123000000, time.UTC)
	receivedAt := occurredAt.Add(250 * time.Millisecond)
	envelope := domain.EventEnvelope{
		ID: uuid.New(), EventID: eventID, Type: "contact.updated", SchemaVersion: 2,
		WorkspaceID: "workspace-1",
		Subject:     domain.EventSubject{Type: "contact", ID: "contact-1", ContactEmail: "person@example.com"},
		Source:      "crm", OccurredAt: occurredAt, ReceivedAt: receivedAt, CorrelationID: correlationID,
		Data: json.RawMessage(`{"changes":{"language":{"new":"fr"}}}`),
	}

	projection, err := NewEventProjection(envelope, receivedAt.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, "workspace-1", projection.WorkspaceID)
	assert.Equal(t, eventID, projection.EventID)
	assert.Equal(t, "contact.updated", projection.EventType)
	assert.Equal(t, "contact", projection.SubjectType)
	assert.Equal(t, "contact-1", projection.SubjectID)
	assert.Equal(t, "person@example.com", projection.ContactEmail)
	assert.Equal(t, occurredAt, projection.OccurredAt)
	assert.Equal(t, receivedAt, projection.ReceivedAt)
	assert.JSONEq(t, string(envelope.Data), projection.PayloadJSON)
}

func TestEventProjectionRejectsInvalidPayload(t *testing.T) {
	envelope := domain.EventEnvelope{
		ID: uuid.New(), EventID: uuid.New(), Type: "contact.updated", SchemaVersion: 1,
		WorkspaceID: "workspace-1", Subject: domain.EventSubject{Type: "contact", ID: "contact-1"},
		Source: "crm", OccurredAt: time.Now(), ReceivedAt: time.Now(), CorrelationID: uuid.New(),
		Data: json.RawMessage(`{"broken":`),
	}
	_, err := NewEventProjection(envelope, time.Now())
	require.Error(t, err)
}

func TestLogicalDedupQueryUsesReplacingMergeTreeFinal(t *testing.T) {
	assert.Contains(t, EventProjectionLogicalQuery, "FINAL")
	assert.Contains(t, EventProjectionLogicalQuery, "event_id")
	assert.Contains(t, EventProjectionTableDDL, "ReplacingMergeTree(projected_at)")
}
