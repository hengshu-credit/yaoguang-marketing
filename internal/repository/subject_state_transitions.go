package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func appendSubjectStateTransitions(ctx context.Context, tx *sql.Tx, w string, previous, current []domain.SubjectStateInterval) error {
	emission := domain.StateTransitionEmissionFrom(ctx)
	if !emission.Enabled || emission.Depth >= 8 {
		return nil
	}
	now := time.Now().UTC()
	for _, change := range domain.SubjectStateTransitions(previous, current, emission.AllowInitial) {
		h := change.Current
		expires := h.ValidFrom.Add(5 * time.Minute)
		if !now.Before(expires) {
			continue
		}
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%s:%s:%s:%s:%d", w, h.SubjectType, h.SubjectID, h.StateKey, h.ValidFrom.UTC().Format(time.RFC3339Nano), h.RuleRevision)))
		data, err := json.Marshal(map[string]interface{}{"state_key": h.StateKey, "value": h.Value, "previous_value": change.Previous, "rule_revision": h.RuleRevision, "processing_status": "accepted", "expires_at": expires, "causation_depth": emission.Depth + 1})
		if err != nil {
			return err
		}
		event := domain.EventEnvelope{ID: uuid.New(), EventID: id, Type: change.EventType, SchemaVersion: 1, WorkspaceID: w, Subject: domain.EventSubject{Type: h.SubjectType, ID: h.SubjectID, CustomerID: h.CustomerID}, Source: "platform.state_projector", OccurredAt: h.ValidFrom, ReceivedAt: now, CorrelationID: id, Data: data}
		if emission.Cause != nil {
			event.CausationID = &emission.Cause.EventID
			event.CorrelationID = emission.Cause.CorrelationID
		}
		if _, err = AppendRealtimeEventTx(ctx, tx, w, event, now); err != nil {
			return err
		}
	}
	return nil
}
