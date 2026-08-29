package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

var (
	ErrPublishConfirmTimeout = errors.New("rabbitmq publish confirm timed out")
	ErrPublishNack           = errors.New("rabbitmq rejected published message")
	ErrPublishChannelClosed  = errors.New("rabbitmq publish channel closed")
	ErrInvalidMessage        = errors.New("invalid broker message")
)

type Confirmation struct {
	Ack bool
}

type Publishing struct {
	Headers       map[string]any
	ContentType   string
	DeliveryMode  uint8
	MessageID     string
	CorrelationID string
	Timestamp     time.Time
	Type          string
	Body          []byte
}

type PublishChannel interface {
	EnableConfirm() error
	Publish(context.Context, string, string, Publishing) error
	Confirms() <-chan Confirmation
	Closed() <-chan error
}

type ConfirmedPublisher struct {
	channel PublishChannel
	timeout time.Duration
	mu      sync.Mutex
}

func NewPublisher(channel PublishChannel, timeout time.Duration) (*ConfirmedPublisher, error) {
	if channel == nil {
		return nil, fmt.Errorf("%w: nil publish channel", ErrInvalidMessage)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("%w: confirm timeout must be positive", ErrInvalidMessage)
	}
	if err := channel.EnableConfirm(); err != nil {
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	return &ConfirmedPublisher{channel: channel, timeout: timeout}, nil
}

func (p *ConfirmedPublisher) Publish(ctx context.Context, message Message) error {
	if message.ID == uuid.Nil {
		return fmt.Errorf("%w: message id is required", ErrInvalidMessage)
	}
	if message.RoutingKey == "" {
		return fmt.Errorf("%w: routing key is required", ErrInvalidMessage)
	}
	if message.Exchange == "" {
		message.Exchange = EventsExchange
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now().UTC()
	}

	headers := make(map[string]any, len(message.Headers)+1)
	for key, value := range message.Headers {
		headers[key] = value
	}
	headers["schema_version"] = message.SchemaVersion

	publishing := Publishing{
		Headers:      headers,
		ContentType:  "application/json",
		DeliveryMode: 2,
		MessageID:    message.ID.String(),
		Timestamp:    message.Timestamp,
		Type:         message.Type,
		Body:         message.Body,
	}
	if message.CorrelationID != uuid.Nil {
		publishing.CorrelationID = message.CorrelationID.String()
	}

	// Confirm channels are ordered per AMQP channel. Serialize publish/wait so a
	// concurrent caller can never consume the previous message's confirmation.
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.channel.Publish(ctx, message.Exchange, message.RoutingKey, publishing); err != nil {
		return fmt.Errorf("publish message %s: %w", message.ID, err)
	}

	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case confirmation, ok := <-p.channel.Confirms():
		if !ok {
			return ErrPublishChannelClosed
		}
		if !confirmation.Ack {
			return ErrPublishNack
		}
		return nil
	case channelErr, ok := <-p.channel.Closed():
		if !ok || channelErr == nil {
			return ErrPublishChannelClosed
		}
		return fmt.Errorf("%w: %v", ErrPublishChannelClosed, channelErr)
	case <-timer.C:
		return ErrPublishConfirmTimeout
	}
}

type amqpPublishChannel struct {
	channel  *amqp091.Channel
	confirms chan Confirmation
	closed   chan error
}

func newAMQPPublishChannel(channel *amqp091.Channel) *amqpPublishChannel {
	return &amqpPublishChannel{channel: channel}
}

func (a *amqpPublishChannel) EnableConfirm() error {
	if err := a.channel.Confirm(false); err != nil {
		return err
	}

	a.confirms = make(chan Confirmation, 1)
	a.closed = make(chan error, 1)
	nativeConfirms := a.channel.NotifyPublish(make(chan amqp091.Confirmation, 1))
	nativeClosed := a.channel.NotifyClose(make(chan *amqp091.Error, 1))
	go func() {
		defer close(a.confirms)
		for confirmation := range nativeConfirms {
			a.confirms <- Confirmation{Ack: confirmation.Ack}
		}
	}()
	go func() {
		defer close(a.closed)
		if channelErr, ok := <-nativeClosed; ok && channelErr != nil {
			a.closed <- channelErr
		}
	}()
	return nil
}

func (a *amqpPublishChannel) Publish(
	ctx context.Context,
	exchange string,
	routingKey string,
	publishing Publishing,
) error {
	return a.channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp091.Publishing{
		Headers:       amqp091.Table(publishing.Headers),
		ContentType:   publishing.ContentType,
		DeliveryMode:  publishing.DeliveryMode,
		MessageId:     publishing.MessageID,
		CorrelationId: publishing.CorrelationID,
		Timestamp:     publishing.Timestamp,
		Type:          publishing.Type,
		Body:          publishing.Body,
	})
}

func (a *amqpPublishChannel) Confirms() <-chan Confirmation { return a.confirms }
func (a *amqpPublishChannel) Closed() <-chan error          { return a.closed }

// RabbitPublisher owns connection lifecycle. A failed attempt invalidates the
// connection and is retried once; stable Message.ID values make that duplicate-
// safe for inbox-based consumers when a confirmation is lost in transit.
type RabbitPublisher struct {
	url            string
	confirmTimeout time.Duration
	mu             sync.Mutex
	connection     *amqp091.Connection
	channel        *amqp091.Channel
	publisher      *ConfirmedPublisher
}

func NewRabbitPublisher(url string, confirmTimeout time.Duration) (*RabbitPublisher, error) {
	if url == "" {
		return nil, errors.New("rabbitmq url is required")
	}
	if confirmTimeout <= 0 {
		return nil, errors.New("rabbitmq confirm timeout must be positive")
	}
	return &RabbitPublisher{url: url, confirmTimeout: confirmTimeout}, nil
}

func (p *RabbitPublisher) Publish(ctx context.Context, message Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := p.connect(); err != nil {
			lastErr = err
			p.invalidate()
			continue
		}
		if err := p.publisher.Publish(ctx, message); err != nil {
			lastErr = err
			p.invalidate()
			if ctx.Err() != nil || errors.Is(err, ErrInvalidMessage) {
				return err
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("publish after reconnect: %w", lastErr)
}

func (p *RabbitPublisher) connect() error {
	if p.connection != nil && !p.connection.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return nil
	}

	connection, err := amqp091.Dial(p.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	publisher, err := NewPublisher(newAMQPPublishChannel(channel), p.confirmTimeout)
	if err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return err
	}
	p.connection = connection
	p.channel = channel
	p.publisher = publisher
	return nil
}

func (p *RabbitPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.invalidate()
}

func (p *RabbitPublisher) invalidate() error {
	var closeErr error
	if p.channel != nil && !p.channel.IsClosed() {
		closeErr = p.channel.Close()
	}
	if p.connection != nil && !p.connection.IsClosed() {
		if err := p.connection.Close(); closeErr == nil {
			closeErr = err
		}
	}
	p.publisher = nil
	p.channel = nil
	p.connection = nil
	return closeErr
}

type AMQPTopologyDeclarer struct {
	Channel *amqp091.Channel
}

func (d AMQPTopologyDeclarer) DeclareExchange(_ context.Context, exchange ExchangeDefinition) error {
	return d.Channel.ExchangeDeclare(
		exchange.Name,
		exchange.Kind,
		exchange.Durable,
		exchange.AutoDelete,
		exchange.Internal,
		false,
		amqp091.Table(exchange.Arguments),
	)
}

func (d AMQPTopologyDeclarer) DeclareQueue(_ context.Context, queue QueueDefinition) error {
	_, err := d.Channel.QueueDeclare(
		queue.Name,
		queue.Durable,
		queue.AutoDelete,
		queue.Exclusive,
		false,
		amqp091.Table(queue.Arguments),
	)
	return err
}

func (d AMQPTopologyDeclarer) BindQueue(_ context.Context, binding BindingDefinition) error {
	return d.Channel.QueueBind(
		binding.Queue,
		binding.RoutingKey,
		binding.Exchange,
		false,
		amqp091.Table(binding.Arguments),
	)
}

type deliveryAcknowledger interface {
	Ack(multiple bool) error
	Nack(multiple bool, requeue bool) error
}

type IncomingDelivery struct {
	Message
	acknowledger deliveryAcknowledger
}

func (d IncomingDelivery) Ack() error {
	return d.acknowledger.Ack(false)
}

func (d IncomingDelivery) Nack(requeue bool) error {
	return d.acknowledger.Nack(false, requeue)
}

// RabbitConsumer uses an explicit prefetch window and manual acknowledgements.
// It reconnects after broker/channel closure until the caller cancels ctx.
type RabbitConsumer struct {
	url            string
	consumerName   string
	prefetch       int
	concurrency    int
	retryPublisher Publisher
	reconnectDelay time.Duration
}

func NewRabbitConsumer(
	url string,
	consumerName string,
	prefetch int,
	concurrency int,
	retryPublisher Publisher,
) (*RabbitConsumer, error) {
	if url == "" {
		return nil, errors.New("rabbitmq url is required")
	}
	if consumerName == "" {
		return nil, errors.New("rabbitmq consumer name is required")
	}
	if prefetch <= 0 {
		return nil, errors.New("rabbitmq prefetch must be positive")
	}
	if concurrency <= 0 {
		return nil, errors.New("rabbitmq consumer concurrency must be positive")
	}
	if retryPublisher == nil {
		return nil, errors.New("rabbitmq retry publisher is required")
	}
	return &RabbitConsumer{
		url:            url,
		consumerName:   consumerName,
		prefetch:       prefetch,
		concurrency:    concurrency,
		retryPublisher: retryPublisher,
		reconnectDelay: time.Second,
	}, nil
}

func (c *RabbitConsumer) Consume(ctx context.Context, queue string, handler Handler) error {
	if queue == "" {
		return errors.New("rabbitmq queue is required")
	}
	if handler == nil {
		return errors.New("rabbitmq handler is required")
	}

	for {
		err := c.consumeSession(ctx, queue, handler)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}

		timer := time.NewTimer(c.reconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *RabbitConsumer) consumeSession(ctx context.Context, queue string, handler Handler) error {
	connection, err := amqp091.Dial(c.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq consumer: %w", err)
	}
	defer connection.Close()

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq consumer channel: %w", err)
	}
	defer channel.Close()

	if err := DefaultTopology().Declare(ctx, AMQPTopologyDeclarer{Channel: channel}); err != nil {
		return err
	}
	if err := channel.Qos(c.prefetch, 0, false); err != nil {
		return fmt.Errorf("set rabbitmq prefetch: %w", err)
	}
	deliveries, err := channel.ConsumeWithContext(
		ctx,
		queue,
		c.consumerName,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("consume rabbitmq queue %s: %w", queue, err)
	}

	semaphore := make(chan struct{}, c.concurrency)
	var workers sync.WaitGroup
	defer workers.Wait()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return ErrPublishChannelClosed
			}
			incoming, err := decodeDelivery(delivery)
			if err != nil {
				_ = delivery.Nack(false, false)
				continue
			}

			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-semaphore }()
				decision := handler(ctx, incoming.Message)
				_ = c.settle(ctx, incoming, decision)
			}()
		}
	}
}

func decodeDelivery(delivery amqp091.Delivery) (IncomingDelivery, error) {
	messageID, err := uuid.Parse(delivery.MessageId)
	if err != nil {
		return IncomingDelivery{}, fmt.Errorf("invalid rabbitmq message_id: %w", err)
	}
	var correlationID uuid.UUID
	if delivery.CorrelationId != "" {
		correlationID, err = uuid.Parse(delivery.CorrelationId)
		if err != nil {
			return IncomingDelivery{}, fmt.Errorf("invalid rabbitmq correlation_id: %w", err)
		}
	}
	schemaVersion := headerInt(delivery.Headers["schema_version"])
	headers := make(map[string]any, len(delivery.Headers))
	for key, value := range delivery.Headers {
		headers[key] = value
	}
	return IncomingDelivery{
		Message: Message{
			ID:            messageID,
			CorrelationID: correlationID,
			Exchange:      delivery.Exchange,
			RoutingKey:    delivery.RoutingKey,
			Type:          delivery.Type,
			SchemaVersion: schemaVersion,
			Timestamp:     delivery.Timestamp,
			Headers:       headers,
			Body:          delivery.Body,
		},
		acknowledger: delivery,
	}, nil
}

func headerInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (c *RabbitConsumer) settle(ctx context.Context, delivery IncomingDelivery, decision DeliveryDecision) error {
	switch decision.Action {
	case Ack:
		return delivery.Ack()
	case DeadLetter:
		return delivery.Nack(false)
	case Retry:
		if !validRetryTier(decision.RetryTier) {
			_ = delivery.Nack(false)
			return fmt.Errorf("invalid retry tier %q", decision.RetryTier)
		}
		message := delivery.Message
		message.Exchange = RetryExchange
		message.RoutingKey = c.consumerName + "." + string(decision.RetryTier)
		if err := c.retryPublisher.Publish(ctx, message); err != nil {
			_ = delivery.Nack(true)
			return fmt.Errorf("publish retry: %w", err)
		}
		return delivery.Ack()
	default:
		_ = delivery.Nack(false)
		return fmt.Errorf("invalid delivery action %d", decision.Action)
	}
}

func validRetryTier(tier RetryTier) bool {
	for _, candidate := range RetryTiers {
		if tier == candidate {
			return true
		}
	}
	return false
}
