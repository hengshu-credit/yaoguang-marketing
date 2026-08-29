package broker

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	EventsExchange = "notifuse.events"
	JobsExchange   = "notifuse.jobs"
	RetryExchange  = "notifuse.retry"
	DeadExchange   = "notifuse.dlx"
)

// Message is the transport-neutral envelope used by relays and workers.
// ID must remain stable across retries so consumers can enforce idempotency.
type Message struct {
	ID            uuid.UUID
	CorrelationID uuid.UUID
	Exchange      string
	RoutingKey    string
	Type          string
	SchemaVersion int
	Timestamp     time.Time
	Headers       map[string]any
	Body          []byte
}

type Publisher interface {
	Publish(context.Context, Message) error
}

type Consumer interface {
	Consume(context.Context, string, Handler) error
}

type Handler func(context.Context, Message) DeliveryDecision

type DeliveryAction uint8

const (
	Ack DeliveryAction = iota
	Retry
	DeadLetter
)

type DeliveryDecision struct {
	Action    DeliveryAction
	RetryTier RetryTier
	Err       error
}

type RetryTier string

const (
	Retry5Seconds  RetryTier = "5s"
	Retry30Seconds RetryTier = "30s"
	Retry5Minutes  RetryTier = "5m"
	Retry30Minutes RetryTier = "30m"
)

var RetryTiers = []RetryTier{
	Retry5Seconds,
	Retry30Seconds,
	Retry5Minutes,
	Retry30Minutes,
}
