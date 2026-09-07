package domain

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

type ConfiguredEventJob struct {
	ID      uuid.UUID
	Token   uuid.UUID
	Payload json.RawMessage
}
type ConfiguredEventQueue interface {
	ClaimConfiguredEvents(context.Context, string, time.Time, time.Duration, int) ([]ConfiguredEventJob, error)
	FinishConfiguredEvent(context.Context, string, ConfiguredEventJob, time.Time, string) error
}
