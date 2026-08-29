package broker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAcknowledger struct {
	acked       int
	nacked      int
	lastRequeue bool
}

func (f *fakeAcknowledger) Ack(bool) error {
	f.acked++
	return nil
}

func (f *fakeAcknowledger) Nack(_ bool, requeue bool) error {
	f.nacked++
	f.lastRequeue = requeue
	return nil
}

type fakePublisher struct {
	err     error
	message Message
	calls   int
}

func (f *fakePublisher) Publish(_ context.Context, message Message) error {
	f.calls++
	f.message = message
	return f.err
}

func TestRetryAcknowledgesOnlyAfterConfirmedRepublish(t *testing.T) {
	acknowledger := &fakeAcknowledger{}
	publisher := &fakePublisher{}
	messageID := uuid.New()
	consumer := &RabbitConsumer{consumerName: "rule", retryPublisher: publisher}

	err := consumer.settle(context.Background(), IncomingDelivery{
		Message:      Message{ID: messageID, RoutingKey: "contact.updated"},
		acknowledger: acknowledger,
	}, DeliveryDecision{Action: Retry, RetryTier: Retry30Seconds})

	require.NoError(t, err)
	assert.Equal(t, 1, publisher.calls)
	assert.Equal(t, RetryExchange, publisher.message.Exchange)
	assert.Equal(t, "rule.30s", publisher.message.RoutingKey)
	assert.Equal(t, messageID, publisher.message.ID)
	assert.Equal(t, 1, acknowledger.acked)
	assert.Zero(t, acknowledger.nacked)
}

func TestRetryPublishFailureRequeuesOriginalDelivery(t *testing.T) {
	acknowledger := &fakeAcknowledger{}
	publisher := &fakePublisher{err: errors.New("confirm lost")}
	consumer := &RabbitConsumer{consumerName: "journey", retryPublisher: publisher}

	err := consumer.settle(context.Background(), IncomingDelivery{
		Message:      Message{ID: uuid.New()},
		acknowledger: acknowledger,
	}, DeliveryDecision{Action: Retry, RetryTier: Retry5Seconds})

	require.ErrorContains(t, err, "confirm lost")
	assert.Zero(t, acknowledger.acked)
	assert.Equal(t, 1, acknowledger.nacked)
	assert.True(t, acknowledger.lastRequeue)
}

func TestDeadLetterNacksWithoutRequeue(t *testing.T) {
	acknowledger := &fakeAcknowledger{}
	consumer := &RabbitConsumer{}

	require.NoError(t, consumer.settle(context.Background(), IncomingDelivery{
		Message:      Message{ID: uuid.New()},
		acknowledger: acknowledger,
	}, DeliveryDecision{Action: DeadLetter}))

	assert.Zero(t, acknowledger.acked)
	assert.Equal(t, 1, acknowledger.nacked)
	assert.False(t, acknowledger.lastRequeue)
}

func TestInvalidRetryTierDeadLetters(t *testing.T) {
	acknowledger := &fakeAcknowledger{}
	consumer := &RabbitConsumer{consumerName: "delivery", retryPublisher: &fakePublisher{}}

	err := consumer.settle(context.Background(), IncomingDelivery{
		Message:      Message{ID: uuid.New()},
		acknowledger: acknowledger,
	}, DeliveryDecision{Action: Retry, RetryTier: "tomorrow"})

	require.Error(t, err)
	assert.Equal(t, 1, acknowledger.nacked)
	assert.False(t, acknowledger.lastRequeue)
}
