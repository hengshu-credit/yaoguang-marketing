package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

type fakeRealtimeComponent struct {
	name       string
	readyErr   error
	started    chan struct{}
	closeOrder *[]string
	mu         *sync.Mutex
}

func (c *fakeRealtimeComponent) Name() string { return c.name }
func (c *fakeRealtimeComponent) Run(ctx context.Context) error {
	if c.started != nil {
		close(c.started)
	}
	<-ctx.Done()
	return ctx.Err()
}
func (c *fakeRealtimeComponent) Ready(context.Context) error { return c.readyErr }
func (c *fakeRealtimeComponent) Close() error {
	if c.closeOrder != nil {
		c.mu.Lock()
		*c.closeOrder = append(*c.closeOrder, c.name)
		c.mu.Unlock()
	}
	return nil
}

type fakeRealtimeFactory struct {
	built    []config.RuntimeCapability
	readyErr map[config.RuntimeCapability]error
	started  map[config.RuntimeCapability]chan struct{}
}

func (f *fakeRealtimeFactory) Build(capability config.RuntimeCapability) (RealtimeComponent, error) {
	f.built = append(f.built, capability)
	return &fakeRealtimeComponent{
		name: string(capability), readyErr: f.readyErr[capability], started: f.started[capability],
	}, nil
}

func TestRealtimeDependenciesBuildExactRoleCapabilityMatrix(t *testing.T) {
	tests := []struct {
		role config.RuntimeRole
		want []config.RuntimeCapability
	}{
		{config.RoleAPI, nil},
		{config.RoleOutboxRelay, []config.RuntimeCapability{config.CapabilityOutboxRelay}},
		{config.RoleRuleWorker, []config.RuntimeCapability{config.CapabilityRule}},
		{config.RoleJourneyWorker, []config.RuntimeCapability{config.CapabilityJourney}},
		{config.RoleDeliveryWorker, []config.RuntimeCapability{config.CapabilityDelivery}},
		{config.RoleAnalyticsWorker, []config.RuntimeCapability{config.CapabilityAnalytics}},
		{config.RoleScheduler, []config.RuntimeCapability{config.CapabilityScheduler}},
		{config.RoleAll, RealtimeWorkerCapabilities},
	}
	for _, test := range tests {
		t.Run(string(test.role), func(t *testing.T) {
			factory := &fakeRealtimeFactory{}
			dependencies, err := NewRealtimeDependencies(test.role, factory)
			require.NoError(t, err)
			require.NotNil(t, dependencies)
			assert.Equal(t, test.want, factory.built)
		})
	}
}

func TestAPIRealtimeReadinessDoesNotDependOnRabbitMQ(t *testing.T) {
	factory := &fakeRealtimeFactory{readyErr: map[config.RuntimeCapability]error{
		config.CapabilityOutboxRelay: errors.New("rabbitmq unavailable"),
	}}
	dependencies, err := NewRealtimeDependencies(config.RoleAPI, factory)
	require.NoError(t, err)
	require.NoError(t, dependencies.Ready(context.Background()))
	assert.Empty(t, factory.built)
}

func TestOutboxReadinessFailsWhenRabbitMQIsUnavailable(t *testing.T) {
	factory := &fakeRealtimeFactory{readyErr: map[config.RuntimeCapability]error{
		config.CapabilityOutboxRelay: errors.New("rabbitmq unavailable"),
	}}
	dependencies, err := NewRealtimeDependencies(config.RoleOutboxRelay, factory)
	require.NoError(t, err)
	err = dependencies.Ready(context.Background())
	require.ErrorContains(t, err, "outbox-relay")
	require.ErrorContains(t, err, "rabbitmq unavailable")
}

func TestRealtimeShutdownStopsComponentsInReverseOrder(t *testing.T) {
	var mu sync.Mutex
	closeOrder := make([]string, 0, 2)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	dependencies := &RealtimeDependencies{components: []RealtimeComponent{
		&fakeRealtimeComponent{name: "client", started: firstStarted, closeOrder: &closeOrder, mu: &mu},
		&fakeRealtimeComponent{name: "consumer", started: secondStarted, closeOrder: &closeOrder, mu: &mu},
	}}
	require.NoError(t, dependencies.Start(context.Background()))
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("consumer did not start")
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("client did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, dependencies.Shutdown(ctx))
	assert.Equal(t, []string{"consumer", "client"}, closeOrder)
}

func TestWorkerRoleStartRunsOwnedComponentWithoutCreatingHTTPServer(t *testing.T) {
	started := make(chan struct{})
	factory := &fakeRealtimeFactory{started: map[config.RuntimeCapability]chan struct{}{
		config.CapabilityOutboxRelay: started,
	}}
	app := NewApp(&config.Config{Realtime: config.RealtimeConfig{
		Role: config.RoleOutboxRelay, Mode: config.RealtimeModePrimary,
	}}, WithRealtimeComponentFactory(factory)).(*App)
	require.NoError(t, app.InitRealtimeDependencies())

	result := make(chan error, 1)
	go func() { result <- app.Start() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("outbox relay component did not start")
	}
	assert.False(t, app.IsServerCreated())
	app.shutdownCancel()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("worker role did not stop with shutdown context")
	}
}
