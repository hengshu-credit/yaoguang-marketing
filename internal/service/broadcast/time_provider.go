package broadcast

import (
	"time"
)

// TimeProvider is an interface that provides time-related functionality
// that can be mocked in tests
type TimeProvider interface {
	// Now returns the current time
	Now() time.Time

	// Since returns the time elapsed since t
	Since(t time.Time) time.Duration
}

// RealTimeProvider is the default implementation of TimeProvider
// that uses the actual system time
type RealTimeProvider struct{}

// Now returns the current time
func (rtp RealTimeProvider) Now() time.Time {
	return time.Now()
}

// Since returns the time elapsed since t
func (rtp RealTimeProvider) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// NewRealTimeProvider creates a new RealTimeProvider
func NewRealTimeProvider() TimeProvider {
	return &RealTimeProvider{}
}
