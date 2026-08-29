package broker

import (
	"context"
	"fmt"
)

type ExchangeDefinition struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	Arguments  map[string]any
}

type QueueDefinition struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	Arguments  map[string]any
	DeadLetter bool
}

type BindingDefinition struct {
	Queue      string
	Exchange   string
	RoutingKey string
	Arguments  map[string]any
}

type Topology struct {
	Exchanges []ExchangeDefinition
	Queues    []QueueDefinition
	Bindings  []BindingDefinition
}

type TopologyDeclarer interface {
	DeclareExchange(context.Context, ExchangeDefinition) error
	DeclareQueue(context.Context, QueueDefinition) error
	BindQueue(context.Context, BindingDefinition) error
}

func (t Topology) Declare(ctx context.Context, declarer TopologyDeclarer) error {
	for _, exchange := range t.Exchanges {
		if err := declarer.DeclareExchange(ctx, exchange); err != nil {
			return fmt.Errorf("declare exchange %s: %w", exchange.Name, err)
		}
	}
	for _, queue := range t.Queues {
		if err := declarer.DeclareQueue(ctx, queue); err != nil {
			return fmt.Errorf("declare queue %s: %w", queue.Name, err)
		}
	}
	for _, binding := range t.Bindings {
		if err := declarer.BindQueue(ctx, binding); err != nil {
			return fmt.Errorf("bind queue %s to %s: %w", binding.Queue, binding.Exchange, err)
		}
	}
	return nil
}

func DefaultTopology() Topology {
	topology := Topology{
		Exchanges: []ExchangeDefinition{
			{Name: EventsExchange, Kind: "topic", Durable: true},
			{Name: JobsExchange, Kind: "topic", Durable: true},
			{Name: RetryExchange, Kind: "topic", Durable: true},
			{Name: DeadExchange, Kind: "topic", Durable: true},
		},
	}

	consumers := []struct {
		name         string
		exchange     string
		routingKey   string
		retryRouting string
	}{
		{name: "rule", exchange: EventsExchange, routingKey: "#", retryRouting: "retry.rule"},
		{name: "journey", exchange: JobsExchange, routingKey: "journey.#", retryRouting: "retry.journey"},
		{name: "delivery", exchange: JobsExchange, routingKey: "delivery.#", retryRouting: "retry.delivery"},
		{name: "analytics", exchange: EventsExchange, routingKey: "#", retryRouting: "retry.analytics"},
	}

	for _, consumer := range consumers {
		queueName := "notifuse." + consumer.name
		topology.Queues = append(topology.Queues, QueueDefinition{
			Name:    queueName,
			Durable: true,
			Arguments: map[string]any{
				"x-queue-type":              "quorum",
				"x-dead-letter-exchange":    DeadExchange,
				"x-dead-letter-routing-key": consumer.name + ".dead",
			},
		})
		topology.Bindings = append(topology.Bindings,
			BindingDefinition{Queue: queueName, Exchange: consumer.exchange, RoutingKey: consumer.routingKey},
			BindingDefinition{Queue: queueName, Exchange: consumer.exchange, RoutingKey: consumer.retryRouting},
		)

		deadQueue := queueName + ".dead"
		topology.Queues = append(topology.Queues, QueueDefinition{
			Name:       deadQueue,
			Durable:    true,
			DeadLetter: true,
			Arguments: map[string]any{
				"x-queue-type": "quorum",
			},
		})
		topology.Bindings = append(topology.Bindings, BindingDefinition{
			Queue: deadQueue, Exchange: DeadExchange, RoutingKey: consumer.name + ".#",
		})

		for _, tier := range RetryTiers {
			retryQueue := fmt.Sprintf("%s.retry.%s", queueName, tier)
			topology.Queues = append(topology.Queues, QueueDefinition{
				Name:    retryQueue,
				Durable: true,
				Arguments: map[string]any{
					"x-queue-type":              "quorum",
					"x-message-ttl":             retryTierMilliseconds(tier),
					"x-dead-letter-exchange":    consumer.exchange,
					"x-dead-letter-routing-key": consumer.retryRouting,
				},
			})
			topology.Bindings = append(topology.Bindings, BindingDefinition{
				Queue: retryQueue, Exchange: RetryExchange, RoutingKey: consumer.name + "." + string(tier),
			})
		}
	}

	return topology
}

func retryTierMilliseconds(tier RetryTier) int64 {
	switch tier {
	case Retry5Seconds:
		return 5_000
	case Retry30Seconds:
		return 30_000
	case Retry5Minutes:
		return 300_000
	case Retry30Minutes:
		return 1_800_000
	default:
		return 0
	}
}
