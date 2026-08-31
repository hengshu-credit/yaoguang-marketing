package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRuntimeRoleRejectsUnknown(t *testing.T) {
	_, err := ParseRuntimeRole("worker")

	require.ErrorContains(t, err, "invalid YAOGUANG_ROLE")
}

func TestLoadRuntimeRolePrefersYaoguangAndSupportsLegacyFallback(t *testing.T) {
	t.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")

	tests := []struct {
		name          string
		yaoguangRole string
		legacyRole   string
		want          RuntimeRole
	}{
		{name: "new variable", yaoguangRole: "api", legacyRole: "all", want: RoleAPI},
		{name: "new variable wins", yaoguangRole: "scheduler", legacyRole: "api", want: RoleScheduler},
		{name: "legacy fallback", yaoguangRole: "", legacyRole: "journey-worker", want: RoleJourneyWorker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("YAOGUANG_ROLE", tt.yaoguangRole)
			t.Setenv("NOTIFUSE_ROLE", tt.legacyRole)

			cfg, err := LoadWithOptions(LoadOptions{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Realtime.Role)
		})
	}
}

func TestLoadRuntimeRoleInvalidYaoguangValueNamesPrimaryVariable(t *testing.T) {
	t.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
	t.Setenv("YAOGUANG_ROLE", "worker")
	t.Setenv("NOTIFUSE_ROLE", "api")

	_, err := LoadWithOptions(LoadOptions{})

	require.ErrorContains(t, err, "invalid YAOGUANG_ROLE")
}

func TestLoadObjectStoreWorkspaceDefaultsFromEnv(t *testing.T) {
	t.Setenv("SECRET_KEY", "test-secret-key-1234567890123456")
	t.Setenv("S3_PROVIDER", "minio")
	t.Setenv("S3_ENDPOINT", "http://minio:9000")
	t.Setenv("S3_PUBLIC_ENDPOINT", "http://localhost:19002")
	t.Setenv("S3_BUCKET", "workspace-assets")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_ACCESS_KEY", "minio-user")
	t.Setenv("S3_SECRET_KEY", "minio-secret")
	t.Setenv("S3_FORCE_PATH_STYLE", "true")

	cfg, err := LoadWithOptions(LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, ObjectStoreConfig{
		Provider:       "minio",
		Endpoint:       "http://minio:9000",
		PublicEndpoint: "http://localhost:19002",
		Bucket:         "workspace-assets",
		Region:         "us-east-1",
		AccessKey:      "minio-user",
		SecretKey:      "minio-secret",
		ForcePathStyle: true,
	}, cfg.Realtime.ObjectStore)
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
		assert.True(t, RuntimeRole("").Runs(capability), "zero role must preserve all capability for direct configs: %s", capability)
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
