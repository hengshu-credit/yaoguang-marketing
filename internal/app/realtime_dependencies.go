package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hengshu-credit/yaoguang-marketing/config"
)

var RealtimeWorkerCapabilities = []config.RuntimeCapability{
	config.CapabilityOutboxRelay,
	config.CapabilityRule,
	config.CapabilityJourney,
	config.CapabilityDelivery,
	config.CapabilityAnalytics,
	config.CapabilityScheduler,
}

type RealtimeComponent interface {
	Name() string
	Run(context.Context) error
	Ready(context.Context) error
	Close() error
}

type RealtimeComponentFactory interface {
	Build(config.RuntimeCapability) (RealtimeComponent, error)
}

type RealtimeDependencies struct {
	components []RealtimeComponent

	mu       sync.Mutex
	started  bool
	cancel   context.CancelFunc
	runErrs  map[string]error
	wait     sync.WaitGroup
	stopOnce sync.Once
}

func NewRealtimeDependencies(role config.RuntimeRole, factory RealtimeComponentFactory) (*RealtimeDependencies, error) {
	if factory == nil {
		return nil, errors.New("realtime component factory is required")
	}
	dependencies := &RealtimeDependencies{runErrs: make(map[string]error)}
	for _, capability := range RealtimeWorkerCapabilities {
		if !role.Runs(capability) {
			continue
		}
		component, err := factory.Build(capability)
		if err != nil {
			return nil, fmt.Errorf("build realtime %s component: %w", capability, err)
		}
		if component == nil {
			return nil, fmt.Errorf("build realtime %s component: factory returned nil", capability)
		}
		dependencies.components = append(dependencies.components, component)
	}
	return dependencies, nil
}

func (d *RealtimeDependencies) Start(parent context.Context) error {
	if parent == nil {
		return errors.New("realtime parent context is required")
	}
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return errors.New("realtime dependencies already started")
	}
	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	d.started = true
	d.mu.Unlock()

	for _, component := range d.components {
		component := component
		d.wait.Add(1)
		go func() {
			defer d.wait.Done()
			if err := component.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				d.mu.Lock()
				d.runErrs[component.Name()] = err
				d.mu.Unlock()
			}
		}()
	}
	return nil
}

func (d *RealtimeDependencies) Ready(ctx context.Context) error {
	var readinessErrors []error
	for _, component := range d.components {
		if err := component.Ready(ctx); err != nil {
			readinessErrors = append(readinessErrors, fmt.Errorf("%s: %w", component.Name(), err))
		}
	}
	d.mu.Lock()
	for name, err := range d.runErrs {
		readinessErrors = append(readinessErrors, fmt.Errorf("%s stopped: %w", name, err))
	}
	d.mu.Unlock()
	return errors.Join(readinessErrors...)
}

func (d *RealtimeDependencies) Shutdown(ctx context.Context) error {
	var shutdownErr error
	d.stopOnce.Do(func() {
		d.mu.Lock()
		cancel := d.cancel
		d.mu.Unlock()
		if cancel != nil {
			cancel()
		}

		closed := make(chan error, 1)
		go func() {
			var closeErr error
			for index := len(d.components) - 1; index >= 0; index-- {
				if err := d.components[index].Close(); err != nil {
					closeErr = errors.Join(closeErr, fmt.Errorf("close %s: %w", d.components[index].Name(), err))
				}
			}
			closed <- closeErr
		}()

		waited := make(chan struct{})
		go func() {
			d.wait.Wait()
			close(waited)
		}()

		for {
			select {
			case err := <-closed:
				shutdownErr = errors.Join(shutdownErr, err)
				closed = nil
			case <-waited:
				if closed != nil {
					select {
					case err := <-closed:
						shutdownErr = errors.Join(shutdownErr, err)
					default:
					}
				}
				return
			case <-ctx.Done():
				shutdownErr = errors.Join(shutdownErr, ctx.Err())
				return
			}
		}
	})
	return shutdownErr
}

func (d *RealtimeDependencies) ComponentNames() []string {
	names := make([]string, len(d.components))
	for index := range d.components {
		names[index] = d.components[index].Name()
	}
	return names
}
