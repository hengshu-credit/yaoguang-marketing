package broker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestRabbitMQRetryAndDeadLetterTopology(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live RabbitMQ test in short mode")
	}
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL is not configured")
	}

	connection, err := amqp091.Dial(url)
	require.NoError(t, err)
	defer connection.Close()
	channel, err := connection.Channel()
	require.NoError(t, err)
	defer channel.Close()

	require.NoError(t, DefaultTopology().Declare(context.Background(), AMQPTopologyDeclarer{Channel: channel}))
	for _, queue := range []string{"notifuse.rule", "notifuse.rule.retry.5s", "notifuse.rule.dead"} {
		_, err = channel.QueuePurge(queue, false)
		require.NoError(t, err)
	}

	mainDeliveries, err := channel.Consume("notifuse.rule", "broker-integration-main", false, false, false, false, nil)
	require.NoError(t, err)
	deadDeliveries, err := channel.Consume("notifuse.rule.dead", "broker-integration-dead", false, false, false, false, nil)
	require.NoError(t, err)

	publisher, err := NewRabbitPublisher(url, 3*time.Second)
	require.NoError(t, err)
	defer publisher.Close()

	deadID := uuid.New()
	require.NoError(t, publisher.Publish(context.Background(), Message{
		ID: deadID, Exchange: EventsExchange, RoutingKey: "contact.updated", SchemaVersion: 1, Body: []byte(`{"kind":"dead"}`),
	}))
	first := receiveDelivery(t, mainDeliveries, 5*time.Second)
	require.Equal(t, deadID.String(), first.MessageId)
	require.NoError(t, first.Nack(false, false))
	dead := receiveDelivery(t, deadDeliveries, 5*time.Second)
	require.Equal(t, deadID.String(), dead.MessageId)
	require.NoError(t, dead.Ack(false))

	retryID := uuid.New()
	require.NoError(t, publisher.Publish(context.Background(), Message{
		ID: retryID, Exchange: RetryExchange, RoutingKey: "rule.5s", SchemaVersion: 1, Body: []byte(`{"kind":"retry"}`),
	}))
	retried := receiveDelivery(t, mainDeliveries, 10*time.Second)
	require.Equal(t, retryID.String(), retried.MessageId)
	require.NoError(t, retried.Ack(false))
}

func receiveDelivery(t *testing.T, deliveries <-chan amqp091.Delivery, timeout time.Duration) amqp091.Delivery {
	t.Helper()
	select {
	case delivery, ok := <-deliveries:
		require.True(t, ok, "RabbitMQ delivery channel closed")
		return delivery
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for RabbitMQ delivery", timeout)
		return amqp091.Delivery{}
	}
}
