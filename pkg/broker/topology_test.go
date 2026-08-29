package broker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyUsesQuorumQueuesAndRetryDeadLetters(t *testing.T) {
	topology := DefaultTopology()
	require.NotEmpty(t, topology.Queues)

	retryQueues := 0
	deadQueues := 0
	for _, queue := range topology.Queues {
		assert.Equal(t, "quorum", queue.Arguments["x-queue-type"], queue.Name)
		if queue.Arguments["x-message-ttl"] != nil {
			retryQueues++
			assert.NotEmpty(t, queue.Arguments["x-dead-letter-exchange"])
		}
		if queue.DeadLetter {
			deadQueues++
		}
	}
	assert.Equal(t, 16, retryQueues, "four consumers need four fixed retry tiers each")
	assert.Equal(t, 4, deadQueues)
}

func TestTopologyDeclaresDurableTopicExchanges(t *testing.T) {
	topology := DefaultTopology()
	require.Len(t, topology.Exchanges, 4)
	for _, exchange := range topology.Exchanges {
		assert.True(t, exchange.Durable)
		assert.Equal(t, "topic", exchange.Kind)
	}
}
