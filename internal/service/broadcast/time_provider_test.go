package broadcast_test

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/internal/service/broadcast/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRealTimeProvider(t *testing.T) {
	t.Run("Now returns current time", func(t *testing.T) {
		// Arrange
		provider := broadcast.NewRealTimeProvider()
		beforeTest := time.Now()

		// Act
		result := provider.Now()

		// Assert
		afterTest := time.Now()
		assert.True(t, !result.Before(beforeTest), "Time from provider should not be before the test started")
		assert.True(t, !result.After(afterTest), "Time from provider should not be after the test finished")
	})

	t.Run("Since returns correct duration", func(t *testing.T) {
		// Arrange
		provider := broadcast.NewRealTimeProvider()
		startTime := time.Now().Add(-100 * time.Millisecond)

		// Act
		duration := provider.Since(startTime)

		// Assert
		assert.True(t, duration >= 100*time.Millisecond, "Duration should be at least 100ms")
		assert.True(t, duration < 1*time.Second, "Duration should be less than 1s (sanity check)")
	})
}

func TestMockTimeProvider(t *testing.T) {
	t.Run("Now returns mocked time", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		mockProvider := mocks.NewMockTimeProvider(ctrl)
		mockProvider.EXPECT().Now().Return(mockTime)

		// Act
		result := mockProvider.Now()

		// Assert
		assert.Equal(t, mockTime, result)
	})

	t.Run("Since returns mocked duration", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		startTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		expectedDuration := 5 * time.Minute

		mockProvider := mocks.NewMockTimeProvider(ctrl)
		mockProvider.EXPECT().Since(startTime).Return(expectedDuration)

		// Act
		result := mockProvider.Since(startTime)

		// Assert
		assert.Equal(t, expectedDuration, result)
	})
}

func TestTimeProviderUsage(t *testing.T) {
	t.Run("demonstrates how to use mock in component tests", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fixedTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		mockProvider := mocks.NewMockTimeProvider(ctrl)

		// Configure the mock
		mockProvider.EXPECT().Now().Return(fixedTime).AnyTimes()
		mockProvider.EXPECT().Since(gomock.Any()).DoAndReturn(func(t time.Time) time.Duration {
			return fixedTime.Sub(t)
		}).AnyTimes()

		// This is where you would pass the mockProvider to components that need time functionality
		// Example:
		// broadcaster := NewBroadcaster(mockProvider)
		// result := broadcaster.ProcessSomething()

		// For this test, we'll just verify the mock works as expected
		assert.Equal(t, fixedTime, mockProvider.Now())

		pastTime := fixedTime.Add(-10 * time.Minute)
		expectedDuration := 10 * time.Minute
		assert.Equal(t, expectedDuration, mockProvider.Since(pastTime))
	})
}
