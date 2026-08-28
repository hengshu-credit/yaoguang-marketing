package broadcast

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// testCircuitBreaker is a simplified implementation of the circuit breaker for testing
type testCircuitBreaker struct {
	failures       int
	threshold      int
	cooldownPeriod time.Duration
	lastFailure    time.Time
	isOpen         bool
	mutex          sync.RWMutex
}

// IsOpen checks if the circuit is open (preventing further calls)
func (cb *testCircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	// If circuit is open, check if cooldown period has passed
	if cb.isOpen {
		if time.Since(cb.lastFailure) > cb.cooldownPeriod {
			// Reset circuit after cooldown
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			cb.isOpen = false
			cb.failures = 0
			cb.mutex.Unlock()
			cb.mutex.RLock()
		}
	}

	return cb.isOpen
}

// RecordSuccess records a successful call
func (cb *testCircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.isOpen = false
}

// RecordFailure records a failed call and opens circuit if threshold is reached
func (cb *testCircuitBreaker) RecordFailure(err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.threshold {
		cb.isOpen = true
	}
}

// TestCircuitBreakerStandalone tests the circuit breaker in isolation
func TestCircuitBreakerStandalone(t *testing.T) {
	// Create a circuit breaker directly
	cb := &testCircuitBreaker{
		threshold:      3,
		cooldownPeriod: 1 * time.Second,
	}

	// Initially circuit should be closed
	assert.False(t, cb.IsOpen())

	// Record failures
	cb.RecordFailure(fmt.Errorf("test error"))
	cb.RecordFailure(fmt.Errorf("test error"))
	assert.False(t, cb.IsOpen(), "Circuit should still be closed after 2 failures")

	// Third failure should open the circuit
	cb.RecordFailure(fmt.Errorf("test error"))
	assert.True(t, cb.IsOpen(), "Circuit should be open after 3 failures")

	// Record success should reset the failure count and close the circuit
	cb.RecordSuccess()
	assert.False(t, cb.IsOpen(), "Circuit should be closed after success")

	// Test cooldown period
	cb.RecordFailure(fmt.Errorf("test error"))
	cb.RecordFailure(fmt.Errorf("test error"))
	cb.RecordFailure(fmt.Errorf("test error")) // This should open the circuit
	assert.True(t, cb.IsOpen(), "Circuit should be open after 3 failures")

	// Wait for cooldown period to expire
	time.Sleep(1100 * time.Millisecond)
	assert.False(t, cb.IsOpen(), "Circuit should be closed after cooldown period")
}

func TestCircuitBreaker(t *testing.T) {
	// Setup a circuit breaker with threshold of 2 failures and 100ms cooldown
	threshold := 2
	cooldown := 100 * time.Millisecond
	cb := NewCircuitBreaker(threshold, cooldown)

	// Test initial state
	assert.False(t, cb.IsOpen(), "Circuit breaker should be closed initially")
	assert.Equal(t, 0, cb.failures, "Initial failure count should be 0")
	assert.Equal(t, threshold, cb.threshold, "Threshold should match the configured value")
	assert.Equal(t, cooldown, cb.cooldownPeriod, "Cooldown period should match the configured value")

	// Test recording failures
	cb.RecordFailure(fmt.Errorf("test error"))
	assert.Equal(t, 1, cb.failures, "Failure count should be incremented")
	assert.False(t, cb.IsOpen(), "Circuit breaker should still be closed after 1 failure")

	cb.RecordFailure(fmt.Errorf("test error"))
	assert.Equal(t, 2, cb.failures, "Failure count should be incremented")
	assert.True(t, cb.IsOpen(), "Circuit breaker should be open after reaching threshold")

	// Test that additional failures don't change the state
	cb.RecordFailure(fmt.Errorf("test error"))
	assert.Equal(t, 3, cb.failures, "Failure count should be incremented again")
	assert.True(t, cb.IsOpen(), "Circuit breaker should remain open")

	// Test recording success
	cb.RecordSuccess()
	assert.Equal(t, 0, cb.failures, "Failure count should be reset to 0")
	assert.False(t, cb.IsOpen(), "Circuit breaker should be closed after success")

	// Test cooldown period
	cb.RecordFailure(fmt.Errorf("test error"))
	cb.RecordFailure(fmt.Errorf("test error")) // This should open the circuit
	assert.True(t, cb.IsOpen(), "Circuit breaker should be open")

	// Store the lastFailure time
	lastFailure := cb.lastFailure

	// Wait for the cooldown period
	time.Sleep(cooldown + 10*time.Millisecond) // Add a little extra time to account for any delay

	// After cooldown, circuit should automatically close on the next status check
	assert.False(t, cb.IsOpen(), "Circuit breaker should be closed after cooldown period")

	// The lastFailure time should remain the same (it's not updated during IsOpen check)
	assert.Equal(t, lastFailure, cb.lastFailure, "Last failure time should not change")
}

func TestCircuitBreaker_CustomConfiguration(t *testing.T) {
	// Test with a higher threshold
	highThresholdCB := NewCircuitBreaker(5, 200*time.Millisecond)

	for i := 0; i < 4; i++ {
		highThresholdCB.RecordFailure(fmt.Errorf("test error"))
		assert.False(t, highThresholdCB.IsOpen(), "Circuit should remain closed until threshold is reached")
	}

	highThresholdCB.RecordFailure(fmt.Errorf("test error"))
	assert.True(t, highThresholdCB.IsOpen(), "Circuit should open after 5 failures")

	// Test circuit stays open during cooldown
	assert.True(t, highThresholdCB.IsOpen(), "Circuit should remain open during cooldown")
}
