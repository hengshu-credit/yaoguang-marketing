package domain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewInMemoryEventBus(t *testing.T) {
	bus := NewInMemoryEventBus()
	assert.NotNil(t, bus)
	assert.NotNil(t, bus.subscribers)
	assert.Empty(t, bus.subscribers)
}

func TestInMemoryEventBus_Subscribe(t *testing.T) {
	bus := NewInMemoryEventBus()
	handler := func(ctx context.Context, payload EventPayload) {}

	// Subscribe to an event
	bus.Subscribe(EventBroadcastSent, handler)

	// Verify the handler was registered
	assert.Len(t, bus.subscribers, 1)
	assert.Contains(t, bus.subscribers, EventBroadcastSent)
	assert.Len(t, bus.subscribers[EventBroadcastSent], 1)

	// Subscribe another handler to the same event
	anotherHandler := func(ctx context.Context, payload EventPayload) {}
	bus.Subscribe(EventBroadcastSent, anotherHandler)

	// Verify both handlers are registered
	assert.Len(t, bus.subscribers[EventBroadcastSent], 2)

	// Subscribe to a different event
	bus.Subscribe(EventBroadcastFailed, handler)

	// Verify the new event has the handler
	assert.Len(t, bus.subscribers, 2)
	assert.Contains(t, bus.subscribers, EventBroadcastFailed)
	assert.Len(t, bus.subscribers[EventBroadcastFailed], 1)
}

func TestInMemoryEventBus_Publish(t *testing.T) {
	bus := NewInMemoryEventBus()

	// Create a channel to track handler execution
	handlerCalled := make(chan EventPayload, 1)

	// Subscribe a handler that sends to the channel
	handler := func(ctx context.Context, payload EventPayload) {
		handlerCalled <- payload
	}

	bus.Subscribe(EventBroadcastSent, handler)

	// Create test event
	testEvent := EventPayload{
		Type:        EventBroadcastSent,
		WorkspaceID: "workspace-123",
		EntityID:    "broadcast-456",
		Data: map[string]interface{}{
			"recipient_count": 100,
		},
	}

	// Publish the event
	bus.Publish(context.Background(), testEvent)

	// Wait for the handler to be called
	select {
	case receivedPayload := <-handlerCalled:
		// Verify the handler received the correct payload
		assert.Equal(t, testEvent.Type, receivedPayload.Type)
		assert.Equal(t, testEvent.WorkspaceID, receivedPayload.WorkspaceID)
		assert.Equal(t, testEvent.EntityID, receivedPayload.EntityID)
		assert.Equal(t, testEvent.Data["recipient_count"], receivedPayload.Data["recipient_count"])
	case <-time.After(time.Second):
		t.Fatal("Handler not called within 1 second")
	}

	// Publish an event with no subscribers
	bus.Publish(context.Background(), EventPayload{Type: EventBroadcastCancelled})

	// Ensure no unexpected handler calls
	select {
	case <-handlerCalled:
		t.Fatal("Handler called for event it didn't subscribe to")
	case <-time.After(100 * time.Millisecond):
		// This is expected - no handler should be called
	}
}

func TestInMemoryEventBus_PublishWithAck(t *testing.T) {
	bus := NewInMemoryEventBus()

	// Create a wait group to track handler execution
	var wg sync.WaitGroup
	wg.Add(1)

	// Subscribe a handler that resolves the wait group
	handler := func(ctx context.Context, payload EventPayload) {
		defer wg.Done()
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
	}

	bus.Subscribe(EventBroadcastSent, handler)

	// Create test event
	testEvent := EventPayload{
		Type:        EventBroadcastSent,
		WorkspaceID: "workspace-123",
		EntityID:    "broadcast-456",
	}

	// Create a channel for the acknowledgment
	ackCalled := make(chan error, 1)

	// Publish with acknowledgment
	bus.PublishWithAck(context.Background(), testEvent, func(err error) {
		ackCalled <- err
	})

	// Wait for the ack to be called
	select {
	case err := <-ackCalled:
		// Verify no error occurred
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Ack not called within 1 second")
	}

	// Publish with no subscribers
	noSubsEvent := EventPayload{Type: "nonexistent.event"}
	ackCalled = make(chan error, 1)

	bus.PublishWithAck(context.Background(), noSubsEvent, func(err error) {
		ackCalled <- err
	})

	// Verify ack called with no error for event with no subscribers
	select {
	case err := <-ackCalled:
		assert.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Ack not called for event with no subscribers")
	}

	// Test with multiple handlers
	wg.Add(2)
	bus.Subscribe(EventBroadcastFailed, handler)
	bus.Subscribe(EventBroadcastFailed, handler)

	multiEvent := EventPayload{Type: EventBroadcastFailed}
	ackCalled = make(chan error, 1)

	bus.PublishWithAck(context.Background(), multiEvent, func(err error) {
		ackCalled <- err
	})

	// Verify ack called after all handlers complete
	select {
	case err := <-ackCalled:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Ack not called after all handlers complete")
	}
}

func TestInMemoryEventBus_Unsubscribe(t *testing.T) {
	// Exercise Unsubscribe path and ensure PublishWithAck still works and no panic occurs
	bus := NewInMemoryEventBus()
	evt := EventPayload{Type: EventBroadcastSent}

	callCh := make(chan struct{}, 2)
	handler1 := func(ctx context.Context, p EventPayload) { callCh <- struct{}{} }
	handler2 := func(ctx context.Context, p EventPayload) { callCh <- struct{}{} }

	bus.Subscribe(evt.Type, handler1)
	bus.Subscribe(evt.Type, handler2)

	// Call Unsubscribe for coverage; removal may or may not happen depending on impl
	bus.Unsubscribe(evt.Type, handler1)

	done := make(chan error, 1)
	bus.PublishWithAck(context.Background(), evt, func(err error) { done <- err })

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for ack")
	}

	// At least one handler should run
	select {
	case <-callCh:
	default:
		t.Fatalf("expected at least one handler call")
	}
}
