package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRuntimeRoleRejectsUnknown(t *testing.T) {
	_, err := ParseRuntimeRole("worker")

	require.ErrorContains(t, err, "invalid NOTIFUSE_ROLE")
}

func TestParseRuntimeRoleAcceptsSupportedValues(t *testing.T) {
	tests := []struct {
		input string
		want  RuntimeRole
	}{
		{input: "all", want: RoleAll},
		{input: "api", want: RoleAPI},
		{input: "outbox-relay", want: RoleOutboxRelay},
		{input: "rule-worker", want: RoleRuleWorker},
		{input: "journey-worker", want: RoleJourneyWorker},
		{input: "delivery-worker", want: RoleDeliveryWorker},
		{input: "analytics-worker", want: RoleAnalyticsWorker},
		{input: "scheduler", want: RoleScheduler},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRuntimeRole(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRealtimeModeRejectsUnknown(t *testing.T) {
	_, err := ParseRealtimeMode("enabled")

	require.ErrorContains(t, err, "invalid REALTIME_MODE")
}

func TestRealtimeConfigPrimaryRequiresRabbitMQ(t *testing.T) {
	cfg := validRealtimeConfig()
	cfg.Mode = RealtimeModePrimary
	cfg.RabbitMQ.URL = ""

	require.ErrorContains(t, cfg.Validate(false), "RABBITMQ_URL")
}

func TestRealtimeConfigProductionPrimaryRejectsExampleRabbitMQCredentials(t *testing.T) {
	cfg := validRealtimeConfig()
	cfg.Mode = RealtimeModePrimary
	cfg.RabbitMQ.URL = "amqp://guest:guest@rabbitmq:5672/"

	require.ErrorContains(t, cfg.Validate(true), "example credentials")
}

func TestRealtimeConfigRejectsHeartbeatNotShorterThanLease(t *testing.T) {
	cfg := validRealtimeConfig()
	cfg.JourneyLease = 20 * time.Second
	cfg.JourneyHeartbeat = 20 * time.Second

	require.ErrorContains(t, cfg.Validate(false), "JOURNEY_HEARTBEAT")
}

func TestRuntimeRoleCapabilityMatrix(t *testing.T) {
	assert.True(t, RoleAPI.Runs(CapabilityHTTP))
	assert.False(t, RoleAPI.Runs(CapabilityJourney))
	assert.True(t, RoleJourneyWorker.Runs(CapabilityJourney))
	assert.False(t, RoleJourneyWorker.Runs(CapabilityHTTP))

	for _, capability := range []RuntimeCapability{
		CapabilityHTTP,
		CapabilityOutboxRelay,
		CapabilityRule,
		CapabilityJourney,
		CapabilityDelivery,
		CapabilityAnalytics,
		CapabilityScheduler,
	} {
		assert.True(t, RoleAll.Runs(capability), "all must run %s", capability)
	}
}

func validRealtimeConfig() RealtimeConfig {
	return RealtimeConfig{
		Role:             RoleAll,
		Mode:             RealtimeModeLegacy,
		JourneyLease:     time.Minute,
		JourneyHeartbeat: 20 * time.Second,
		OutboxBatchSize:  200,
		OutboxLease:      30 * time.Second,
		RabbitMQ: RabbitMQConfig{
			URL:                   "amqp://guest:guest@rabbitmq:5672/",
			Prefetch:              100,
			PublishConfirmTimeout: 5 * time.Second,
		},
		ClickHouse: ClickHouseConfig{
			BatchSize:     1000,
			FlushInterval: time.Second,
		},
	}
}
