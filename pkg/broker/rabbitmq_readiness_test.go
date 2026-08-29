package broker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEnsureRabbitMQTopologyValidatesInputBeforeDial(t *testing.T) {
	require.ErrorContains(t, EnsureRabbitMQTopology(context.Background(), "", time.Second), "url is required")
	require.ErrorContains(t, EnsureRabbitMQTopology(context.Background(), "amqp://localhost", 0), "timeout must be positive")
}
