package broker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublishChannel struct {
	confirm        chan Confirmation
	closed         chan error
	confirmErr     error
	publishErr     error
	mu             sync.Mutex
	lastPublishing Publishing
}

func (f *fakePublishChannel) EnableConfirm() error { return f.confirmErr }

func (f *fakePublishChannel) Publish(_ context.Context, _ string, _ string, publishing Publishing) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPublishing = publishing
	return f.publishErr
}

func (f *fakePublishChannel) Confirms() <-chan Confirmation { return f.confirm }

func (f *fakePublishChannel) Closed() <-chan error { return f.closed }

func TestPublisherReturnsErrorWhenConfirmTimesOut(t *testing.T) {
	channel := &fakePublishChannel{
		confirm: make(chan Confirmation),
		closed:  make(chan error),
	}
	publisher, err := NewPublisher(channel, 10*time.Millisecond)
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), Message{ID: uuid.New(), RoutingKey: "contact.updated"})
	require.ErrorIs(t, err, ErrPublishConfirmTimeout)
}

func TestPublisherRejectsNegativeConfirm(t *testing.T) {
	channel := &fakePublishChannel{
		confirm: make(chan Confirmation, 1),
		closed:  make(chan error),
	}
	channel.confirm <- Confirmation{Ack: false}
	publisher, err := NewPublisher(channel, time.Second)
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), Message{ID: uuid.New(), RoutingKey: "contact.updated"})
	require.ErrorIs(t, err, ErrPublishNack)
}

func TestPublisherUsesPersistentJSONMessageIdentity(t *testing.T) {
	channel := &fakePublishChannel{
		confirm: make(chan Confirmation, 1),
		closed:  make(chan error),
	}
	channel.confirm <- Confirmation{Ack: true}
	publisher, err := NewPublisher(channel, time.Second)
	require.NoError(t, err)

	messageID := uuid.New()
	correlationID := uuid.New()
	require.NoError(t, publisher.Publish(context.Background(), Message{
		ID:            messageID,
		CorrelationID: correlationID,
		RoutingKey:    "contact.updated",
		SchemaVersion: 1,
		Body:          []byte(`{"ok":true}`),
	}))

	channel.mu.Lock()
	publishing := channel.lastPublishing
	channel.mu.Unlock()
	assert.Equal(t, uint8(2), publishing.DeliveryMode)
	assert.Equal(t, "application/json", publishing.ContentType)
	assert.Equal(t, messageID.String(), publishing.MessageID)
	assert.Equal(t, correlationID.String(), publishing.CorrelationID)
	assert.Equal(t, 1, publishing.Headers["schema_version"])
}

func TestPublisherReturnsChannelClosure(t *testing.T) {
	channel := &fakePublishChannel{
		confirm: make(chan Confirmation),
		closed:  make(chan error, 1),
	}
	channel.closed <- errors.New("connection lost")
	publisher, err := NewPublisher(channel, time.Second)
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), Message{ID: uuid.New(), RoutingKey: "contact.updated"})
	require.ErrorContains(t, err, "connection lost")
}
