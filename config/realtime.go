package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// RuntimeRole selects the independently scalable responsibility run by this
// process. RoleAll preserves the existing single-process deployment model.
type RuntimeRole string

const (
	RoleAll             RuntimeRole = "all"
	RoleAPI             RuntimeRole = "api"
	RoleOutboxRelay     RuntimeRole = "outbox-relay"
	RoleRuleWorker      RuntimeRole = "rule-worker"
	RoleJourneyWorker   RuntimeRole = "journey-worker"
	RoleDeliveryWorker  RuntimeRole = "delivery-worker"
	RoleAnalyticsWorker RuntimeRole = "analytics-worker"
	RoleScheduler       RuntimeRole = "scheduler"
)

// RealtimeMode controls migration from the legacy trigger path to the durable
// realtime runtime.
type RealtimeMode string

const (
	RealtimeModeLegacy  RealtimeMode = "legacy"
	RealtimeModeShadow  RealtimeMode = "shadow"
	RealtimeModePrimary RealtimeMode = "primary"
)

// RuntimeCapability names a process responsibility without coupling config to
// application implementations.
type RuntimeCapability string

const (
	CapabilityHTTP        RuntimeCapability = "http"
	CapabilityOutboxRelay RuntimeCapability = "outbox-relay"
	CapabilityRule        RuntimeCapability = "rule"
	CapabilityJourney     RuntimeCapability = "journey"
	CapabilityDelivery    RuntimeCapability = "delivery"
	CapabilityAnalytics   RuntimeCapability = "analytics"
	CapabilityScheduler   RuntimeCapability = "scheduler"
)

type RabbitMQConfig struct {
	URL                   string
	Prefetch              int
	PublishConfirmTimeout time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type ClickHouseConfig struct {
	Addr          string
	Database      string
	Username      string
	Password      string
	BatchSize     int
	FlushInterval time.Duration
}

type ObjectStoreConfig struct {
	Endpoint       string
	Bucket         string
	Region         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
}

type RealtimeConfig struct {
	Role             RuntimeRole
	Mode             RealtimeMode
	RabbitMQ         RabbitMQConfig
	Redis            RedisConfig
	ClickHouse       ClickHouseConfig
	ObjectStore      ObjectStoreConfig
	JourneyLease     time.Duration
	JourneyHeartbeat time.Duration
	OutboxBatchSize  int
	OutboxLease      time.Duration
}

func ParseRuntimeRole(value string) (RuntimeRole, error) {
	role := RuntimeRole(value)
	switch role {
	case RoleAll, RoleAPI, RoleOutboxRelay, RoleRuleWorker, RoleJourneyWorker,
		RoleDeliveryWorker, RoleAnalyticsWorker, RoleScheduler:
		return role, nil
	default:
		return "", fmt.Errorf("invalid NOTIFUSE_ROLE %q", value)
	}
}

func ParseRealtimeMode(value string) (RealtimeMode, error) {
	mode := RealtimeMode(value)
	switch mode {
	case RealtimeModeLegacy, RealtimeModeShadow, RealtimeModePrimary:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid REALTIME_MODE %q", value)
	}
}

// Runs reports whether a role owns a capability. Unknown capabilities are
// rejected even for RoleAll so wiring mistakes fail closed.
func (r RuntimeRole) Runs(capability RuntimeCapability) bool {
	// The zero value preserves pre-role behavior for tests and embedders that
	// construct Config directly. LoadConfig still rejects an explicitly invalid
	// NOTIFUSE_ROLE before an application is created.
	if r == "" || r == RoleAll {
		switch capability {
		case CapabilityHTTP, CapabilityOutboxRelay, CapabilityRule, CapabilityJourney,
			CapabilityDelivery, CapabilityAnalytics, CapabilityScheduler:
			return true
		default:
			return false
		}
	}

	switch capability {
	case CapabilityHTTP:
		return r == RoleAPI
	case CapabilityOutboxRelay:
		return r == RoleOutboxRelay
	case CapabilityRule:
		return r == RoleRuleWorker
	case CapabilityJourney:
		return r == RoleJourneyWorker
	case CapabilityDelivery:
		return r == RoleDeliveryWorker
	case CapabilityAnalytics:
		return r == RoleAnalyticsWorker
	case CapabilityScheduler:
		return r == RoleScheduler
	default:
		return false
	}
}

func (c RealtimeConfig) Validate(production bool) error {
	if _, err := ParseRuntimeRole(string(c.Role)); err != nil {
		return err
	}
	if _, err := ParseRealtimeMode(string(c.Mode)); err != nil {
		return err
	}
	if c.JourneyLease <= 0 {
		return fmt.Errorf("JOURNEY_LEASE must be positive")
	}
	if c.JourneyHeartbeat <= 0 || c.JourneyHeartbeat >= c.JourneyLease {
		return fmt.Errorf("JOURNEY_HEARTBEAT must be positive and shorter than JOURNEY_LEASE")
	}
	if c.OutboxBatchSize <= 0 {
		return fmt.Errorf("OUTBOX_BATCH_SIZE must be positive")
	}
	if c.OutboxLease <= 0 {
		return fmt.Errorf("OUTBOX_LEASE must be positive")
	}
	if c.RabbitMQ.Prefetch <= 0 {
		return fmt.Errorf("RABBITMQ_PREFETCH must be positive")
	}
	if c.RabbitMQ.PublishConfirmTimeout <= 0 {
		return fmt.Errorf("RABBITMQ_PUBLISH_CONFIRM_TIMEOUT must be positive")
	}
	if c.ClickHouse.BatchSize <= 0 {
		return fmt.Errorf("CLICKHOUSE_BATCH_SIZE must be positive")
	}
	if c.ClickHouse.FlushInterval <= 0 {
		return fmt.Errorf("CLICKHOUSE_FLUSH_INTERVAL must be positive")
	}
	if c.Redis.DB < 0 {
		return fmt.Errorf("REDIS_DB cannot be negative")
	}

	if c.Mode == RealtimeModeLegacy {
		return nil
	}
	if c.RabbitMQ.URL == "" {
		return fmt.Errorf("RABBITMQ_URL is required when REALTIME_MODE=%s", c.Mode)
	}
	rabbitURL, err := url.Parse(c.RabbitMQ.URL)
	if err != nil || (rabbitURL.Scheme != "amqp" && rabbitURL.Scheme != "amqps") || rabbitURL.Host == "" {
		return fmt.Errorf("RABBITMQ_URL must be a valid amqp or amqps URL")
	}
	if production && c.Mode == RealtimeModePrimary && rabbitURL.User != nil {
		password, _ := rabbitURL.User.Password()
		username := rabbitURL.User.Username()
		if strings.EqualFold(username, "guest") || strings.EqualFold(password, "guest") {
			return fmt.Errorf("RABBITMQ_URL must not use example credentials in production primary mode")
		}
	}
	if c.Role.Runs(CapabilityAnalytics) {
		if strings.TrimSpace(c.ClickHouse.Addr) == "" {
			return fmt.Errorf("CLICKHOUSE_ADDR is required for analytics capability")
		}
		if strings.TrimSpace(c.ClickHouse.Database) == "" {
			return fmt.Errorf("CLICKHOUSE_DATABASE is required for analytics capability")
		}
	}

	return nil
}
