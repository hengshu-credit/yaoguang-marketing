package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/eventanalytics"
)

type runtimeComponent struct {
	name    string
	run     func(context.Context) error
	ready   func(context.Context) error
	closeFn func() error
}

func (c *runtimeComponent) Name() string { return c.name }
func (c *runtimeComponent) Run(ctx context.Context) error {
	if c.run == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	return c.run(ctx)
}
func (c *runtimeComponent) Ready(ctx context.Context) error {
	if c.ready == nil {
		return nil
	}
	return c.ready(ctx)
}
func (c *runtimeComponent) Close() error {
	if c.closeFn == nil {
		return nil
	}
	return c.closeFn()
}

type appRealtimeComponentFactory struct {
	app                *App
	realtimeRepository *repository.RealtimePostgresRepository
	inboxRepository    domain.ConsumerInboxRepository
	workerID           string

	publisherMu sync.Mutex
	publisher   *broker.RabbitPublisher
}

func newAppRealtimeComponentFactory(app *App) (*appRealtimeComponentFactory, error) {
	if app == nil || app.workspaceRepo == nil || app.automationRepo == nil || app.automationExecutor == nil {
		return nil, errors.New("realtime runtime requires initialized repositories and automation executor")
	}
	realtimeRepository := repository.NewRealtimeRepository(app.workspaceRepo)
	var inboxRepository domain.ConsumerInboxRepository = realtimeRepository
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "notifuse"
	}
	return &appRealtimeComponentFactory{
		app: app, realtimeRepository: realtimeRepository, inboxRepository: inboxRepository,
		workerID: hostname + "-" + uuid.NewString(),
	}, nil
}

func (f *appRealtimeComponentFactory) rabbitPublisher() (*broker.RabbitPublisher, error) {
	f.publisherMu.Lock()
	defer f.publisherMu.Unlock()
	if f.publisher != nil {
		return f.publisher, nil
	}
	publisher, err := broker.NewRabbitPublisher(
		f.app.config.Realtime.RabbitMQ.URL,
		f.app.config.Realtime.RabbitMQ.PublishConfirmTimeout,
	)
	if err != nil {
		return nil, err
	}
	f.publisher = publisher
	return publisher, nil
}

func (f *appRealtimeComponentFactory) rabbitReady(ctx context.Context) error {
	timeout := f.app.config.Realtime.RabbitMQ.PublishConfirmTimeout
	if timeout < time.Second {
		timeout = time.Second
	}
	return broker.EnsureRabbitMQTopology(ctx, f.app.config.Realtime.RabbitMQ.URL, timeout)
}

func (f *appRealtimeComponentFactory) closePublisher() error {
	f.publisherMu.Lock()
	defer f.publisherMu.Unlock()
	if f.publisher == nil {
		return nil
	}
	return f.publisher.Close()
}

func (f *appRealtimeComponentFactory) newConsumer(name string) (*broker.RabbitConsumer, error) {
	publisher, err := f.rabbitPublisher()
	if err != nil {
		return nil, err
	}
	prefetch := f.app.config.Realtime.RabbitMQ.Prefetch
	return broker.NewRabbitConsumer(
		f.app.config.Realtime.RabbitMQ.URL, name, prefetch, prefetch, publisher,
	)
}

func (f *appRealtimeComponentFactory) Build(capability config.RuntimeCapability) (RealtimeComponent, error) {
	switch capability {
	case config.CapabilityOutboxRelay:
		publisher, err := f.rabbitPublisher()
		if err != nil {
			return nil, err
		}
		relay, err := service.NewOutboxRelay(
			repository.NewWorkspaceCursorRepository(f.app.workspaceRepo),
			f.realtimeRepository, publisher, f.workerID,
			f.app.config.Realtime.OutboxBatchSize, f.app.config.Realtime.OutboxLease,
		)
		if err != nil {
			return nil, err
		}
		return &runtimeComponent{
			name: string(capability), run: relay.Run, ready: f.rabbitReady, closeFn: f.closePublisher,
		}, nil

	case config.CapabilityRule:
		consumer, err := f.newConsumer("rule")
		if err != nil {
			return nil, err
		}
		worker, err := service.NewRuleWorker(
			f.realtimeRepository, f.app.config.Realtime.Mode, f.app.config.Realtime.OutboxLease,
		)
		if err != nil {
			return nil, err
		}
		return f.consumerComponent(capability, consumer, "notifuse.rule", worker.HandleDelivery), nil

	case config.CapabilityJourney:
		consumer, err := f.newConsumer("journey")
		if err != nil {
			return nil, err
		}
		worker, err := service.NewJourneyWorker(
			f.app.automationRepo, f.app.automationExecutor, f.workerID, f.app.config.Realtime.JourneyLease,
		)
		if err != nil {
			return nil, err
		}
		return f.consumerComponent(capability, consumer, "notifuse.journey", worker.HandleDelivery), nil

	case config.CapabilityDelivery:
		consumer, err := f.newConsumer("delivery")
		if err != nil {
			return nil, err
		}
		worker, err := service.NewRealtimeDeliveryWorker(
			f.inboxRepository, f.realtimeRepository, f.app.automationExecutor, f.app.config.Realtime.OutboxLease,
		)
		if err != nil {
			return nil, err
		}
		return f.consumerComponent(capability, consumer, "notifuse.delivery", worker.HandleDelivery), nil

	case config.CapabilityAnalytics:
		return f.buildAnalyticsComponent(capability)

	case config.CapabilityScheduler:
		publisher, err := f.rabbitPublisher()
		if err != nil {
			return nil, err
		}
		scheduler, err := service.NewRealtimeJourneyScheduler(
			f.app.automationRepo, publisher,
			f.app.config.AutomationScheduler.Interval, f.app.config.AutomationScheduler.BatchSize,
		)
		if err != nil {
			return nil, err
		}
		return &runtimeComponent{
			name: string(capability), run: scheduler.Run, ready: f.rabbitReady, closeFn: f.closePublisher,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported realtime capability %q", capability)
	}
}

func (f *appRealtimeComponentFactory) consumerComponent(
	capability config.RuntimeCapability,
	consumer *broker.RabbitConsumer,
	queue string,
	handler broker.Handler,
) RealtimeComponent {
	return &runtimeComponent{
		name: string(capability),
		run: func(ctx context.Context) error {
			return consumer.Consume(ctx, queue, handler)
		},
		ready: f.rabbitReady, closeFn: f.closePublisher,
	}
}

func (f *appRealtimeComponentFactory) buildAnalyticsComponent(capability config.RuntimeCapability) (RealtimeComponent, error) {
	consumer, err := f.newConsumer("analytics")
	if err != nil {
		return nil, err
	}
	cfg := f.app.config.Realtime.ClickHouse
	addresses := make([]string, 0, 1)
	for _, address := range strings.Split(cfg.Addr, ",") {
		if trimmed := strings.TrimSpace(address); trimmed != "" {
			addresses = append(addresses, trimmed)
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("clickhouse address is required for analytics worker")
	}
	store, err := eventanalytics.OpenClickHouseStore(&clickhouse.Options{
		Addr:        addresses,
		Auth:        clickhouse.Auth{Database: cfg.Database, Username: cfg.Username, Password: cfg.Password},
		DialTimeout: 5 * time.Second,
	}, cfg.Database)
	if err != nil {
		return nil, err
	}
	worker, err := service.NewAnalyticsWorker(store, f.inboxRepository, f.app.config.Realtime.OutboxLease)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	batcher, err := service.NewAnalyticsBatcher(worker, cfg.BatchSize, cfg.FlushInterval)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return &runtimeComponent{
		name: string(capability),
		run: func(ctx context.Context) error {
			if err := store.EnsureSchema(ctx); err != nil {
				return err
			}
			batcherDone := make(chan struct{})
			go func() {
				defer close(batcherDone)
				batcher.Run(ctx)
			}()
			consumeErr := consumer.Consume(ctx, "notifuse.analytics", batcher.HandleDelivery)
			<-batcherDone
			return consumeErr
		},
		ready: func(ctx context.Context) error {
			return errors.Join(store.Ping(ctx), f.rabbitReady(ctx))
		},
		closeFn: func() error { return errors.Join(store.Close(), f.closePublisher()) },
	}, nil
}
