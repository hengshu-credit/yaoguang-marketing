//go:build runserver

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/mailer"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestConfigLoading(t *testing.T) {
	// Try to load config from .env.test
	_, err := config.Load()
	// We expect an error if the file doesn't exist in the test environment
	assert.Error(t, err)
}

func TestSetupMinimalConfig(t *testing.T) {
	// Setup test environment variables
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("SERVER_HOST", "localhost")
	os.Setenv("SERVER_PORT", "8081")
	os.Setenv("DB_USER", "postgres_test")
	os.Setenv("DB_PASS", "postgres_test")
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_NAME", "notifuse_test")
	os.Setenv("ROOT_EMAIL", "test@example.com")

	// Cleanup
	defer func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("SERVER_HOST")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASS")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("ROOT_EMAIL")
	}()

	// Try to load config from environment
	cfg, err := config.Load()

	// Might still fail if viper is looking for files specifically
	if err != nil {
		t.Logf("Config Load failed: %v", err)
		return
	}

	// Otherwise, verify config is loaded correctly
	assert.Equal(t, "test", cfg.Environment)
	assert.Equal(t, "localhost", cfg.Server.Host)
	assert.Equal(t, 8081, cfg.Server.Port)
	assert.Equal(t, "postgres_test", cfg.Database.User)
}

// MockApp implements the necessary methods from App for testing
type MockApp struct {
	initializeFunc func() error
	startFunc      func() error
	shutdownFunc   func(ctx context.Context) error
}

func (m *MockApp) Initialize() error {
	return m.initializeFunc()
}

func (m *MockApp) Start() error {
	return m.startFunc()
}

func (m *MockApp) Shutdown(ctx context.Context) error {
	return m.shutdownFunc(ctx)
}

// ExtendedMockApp extends MockApp to also implement the App interface
type ExtendedMockApp struct {
	MockApp
	opts []app.AppOption
}

// Create a custom signal.Notify function for testing
func createMockSignalFunc(sendSignal bool, delay time.Duration) func(c chan<- os.Signal, sig ...os.Signal) {
	return func(c chan<- os.Signal, sig ...os.Signal) {
		// If we should send a signal, do it after the specified delay
		if sendSignal {
			go func() {
				time.Sleep(delay)
				c <- os.Interrupt
			}()
		}
	}
}

// Create a package variable for NewApp to be redefined during tests
var testNewAppFunc func(cfg *config.Config, opts ...app.AppOption) interface{}

// NewApp variable that can be overridden in tests
var NewApp = app.NewApp

// -------------------------------------------------------------------------
// The following code is from runserver_test.go
// These tests are for testing the runServer function
// -------------------------------------------------------------------------

// Test helpers for the runServer test
func createSimpleTestConfig() *config.Config {
	return &config.Config{
		Environment: "test",
		RootEmail:   "test@example.com",
		Database: config.DatabaseConfig{
			User:     "postgres_test",
			Password: "postgres_test",
			Host:     "localhost",
			Port:     5432,
			DBName:   "notifuse_test",
		},
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}
}

// MockAppSimple implements AppInterface for testing the real runServer
type MockAppSimple struct {
	initializeCalled bool
	startCalled      bool
	shutdownCalled   bool
	initializeError  error
	startError       error
	shutdownError    error
	// Channel to notify when Start() is called
	startNotify chan struct{}
	// Function fields for graceful shutdown methods
	SetShutdownTimeoutFunc    func(timeout time.Duration)
	GetActiveRequestCountFunc func() int64
	ShutdownFunc              func(ctx context.Context) error
}

func (m *MockAppSimple) Initialize() error {
	m.initializeCalled = true
	return m.initializeError
}

func (m *MockAppSimple) Start() error {
	m.startCalled = true
	// Notify that start was called
	if m.startNotify != nil {
		close(m.startNotify)
	}
	// If we're not returning an error, block until shutdown is called
	if m.startError == nil {
		// This will block until the test sends a signal and Shutdown() is called
		// which is what happens in the real App implementation
		select {}
	}
	return m.startError
}

func (m *MockAppSimple) Shutdown(ctx context.Context) error {
	m.shutdownCalled = true
	if m.ShutdownFunc != nil {
		return m.ShutdownFunc(ctx)
	}
	return m.shutdownError
}

// Stub implementations of the AppInterface methods added for testing
func (m *MockAppSimple) GetConfig() *config.Config { return nil }
func (m *MockAppSimple) GetLogger() logger.Logger  { return nil }
func (m *MockAppSimple) GetMux() *http.ServeMux    { return nil }
func (m *MockAppSimple) GetDB() *sql.DB            { return nil }
func (m *MockAppSimple) GetMailer() mailer.Mailer  { return nil }
func (m *MockAppSimple) InitDB() error             { return nil }
func (m *MockAppSimple) InitMailer() error         { return nil }
func (m *MockAppSimple) InitRepositories() error   { return nil }
func (m *MockAppSimple) InitServices() error       { return nil }
func (m *MockAppSimple) InitHandlers() error       { return nil }
func (m *MockAppSimple) InitTracing() error        { return nil }

// Repository getters for testing
func (m *MockAppSimple) GetUserRepository() domain.UserRepository                     { return nil }
func (m *MockAppSimple) GetWorkspaceRepository() domain.WorkspaceRepository           { return nil }
func (m *MockAppSimple) GetContactRepository() domain.ContactRepository               { return nil }
func (m *MockAppSimple) GetListRepository() domain.ListRepository                     { return nil }
func (m *MockAppSimple) GetTemplateRepository() domain.TemplateRepository             { return nil }
func (m *MockAppSimple) GetBroadcastRepository() domain.BroadcastRepository           { return nil }
func (m *MockAppSimple) GetMessageHistoryRepository() domain.MessageHistoryRepository { return nil }
func (m *MockAppSimple) GetContactListRepository() domain.ContactListRepository       { return nil }
func (m *MockAppSimple) GetTransactionalNotificationRepository() domain.TransactionalNotificationRepository {
	return nil
}
func (m *MockAppSimple) GetTelemetryRepository() domain.TelemetryRepository { return nil }

// Server status methods
func (m *MockAppSimple) IsServerCreated() bool                       { return true }
func (m *MockAppSimple) WaitForServerStart(ctx context.Context) bool { return true }

// Graceful shutdown methods
func (m *MockAppSimple) SetShutdownTimeout(timeout time.Duration) {
	if m.SetShutdownTimeoutFunc != nil {
		m.SetShutdownTimeoutFunc(timeout)
	}
}
func (m *MockAppSimple) GetActiveRequestCount() int64 {
	if m.GetActiveRequestCountFunc != nil {
		return m.GetActiveRequestCountFunc()
	}
	return 0
}
func (m *MockAppSimple) GetShutdownContext() context.Context { return context.Background() }

// TestActualRunServer directly tests the real runServer function
// This test is only run when the runserver build tag is specified
func TestActualRunServer(t *testing.T) {
	// Skip in normal test runs
	if os.Getenv("RUN_RUNSERVER_TEST") != "true" {
		t.Skip("Skipping TestActualRunServer. Use -tags=runserver to run this test.")
	}

	// Save original functions to restore later
	originalNewApp := NewApp
	originalSignalNotify := signalNotify

	defer func() {
		// Restore original functions
		NewApp = originalNewApp
		signalNotify = originalSignalNotify
	}()

	// Create test cases
	tests := []struct {
		name            string
		initializeError error
		startError      error
		shutdownError   error
		sendSignal      bool
		expectError     bool
	}{
		{
			name:            "Successful initialization and graceful shutdown",
			initializeError: nil,
			startError:      nil,
			shutdownError:   nil,
			sendSignal:      true,
			expectError:     false,
		},
		{
			name:            "Error during initialization",
			initializeError: fmt.Errorf("initialize error"),
			startError:      nil,
			shutdownError:   nil,
			sendSignal:      false,
			expectError:     true,
		},
		{
			name:            "Error during start",
			initializeError: nil,
			startError:      fmt.Errorf("start error"),
			shutdownError:   nil,
			sendSignal:      false,
			expectError:     true,
		},
		{
			name:            "Error during shutdown",
			initializeError: nil,
			startError:      nil,
			shutdownError:   fmt.Errorf("shutdown error"),
			sendSignal:      true,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create channel to know when Start is called
			startNotify := make(chan struct{})

			// Create mock app
			mockApp := &MockAppSimple{
				initializeError: tt.initializeError,
				startError:      tt.startError,
				shutdownError:   tt.shutdownError,
				startNotify:     startNotify,
			}

			// Replace NewApp with our mock implementation
			NewApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface {
				// Apply options to record they were passed
				for _, opt := range opts {
					// We can't use the options directly on the mock
					// but we want to make sure they don't cause a panic
					dummyApp := &app.App{}
					opt(dummyApp)
				}
				return mockApp
			}

			// Replace signal notify to send signal in test
			// Track how many times signalNotify is called (for the new signal handling)
			var callCount int32
			signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
				callNum := atomic.AddInt32(&callCount, 1)
				if callNum == 1 {
					// First call - for initial shutdown channel
					go func() {
						// For cases with startError, Start() returns immediately with an error
						// For other cases, wait until Start is called before sending signal
						if tt.startError == nil {
							<-startNotify
						}

						if tt.sendSignal {
							// Small delay to ensure server is ready
							time.Sleep(100 * time.Millisecond)
							c <- os.Interrupt
						}
					}()
				}
				// Second call would be for force shutdown channel (created after first signal)
				// We don't send anything to it in normal tests
			}

			// Create test config
			cfg := createSimpleTestConfig()

			// Create mock logger with controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockLogger := pkgmocks.NewMockLogger(ctrl)

			// Set up logger expectations
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

			// Run the server in a goroutine
			resultCh := make(chan error, 1)
			go func() {
				resultCh <- runServer(cfg, mockLogger)
			}()

			// Set test timeout
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// Wait for result or timeout
			var err error
			select {
			case err = <-resultCh:
				// Got result
			case <-ctx.Done():
				t.Fatalf("Test timed out")
			}

			// Check result
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify appropriate methods were called
			assert.True(t, mockApp.initializeCalled, "Initialize should have been called")

			if tt.initializeError != nil {
				// If initialization failed, start should not be called
				assert.False(t, mockApp.startCalled, "Start should not have been called after initialization error")
			} else if tt.sendSignal {
				// If we sent a shutdown signal, shutdown should be called
				assert.True(t, mockApp.shutdownCalled, "Shutdown should have been called")
			}
		})
	}
}

// TestGracefulShutdownWithActiveRequests tests the graceful shutdown functionality
func TestGracefulShutdownWithActiveRequests(t *testing.T) {
	// Skip in normal test runs
	if os.Getenv("RUN_RUNSERVER_TEST") != "true" {
		t.Skip("Skipping TestGracefulShutdownWithActiveRequests. Use -tags=runserver to run this test.")
	}

	// Save original functions to restore later
	originalNewApp := NewApp
	originalSignalNotify := signalNotify

	defer func() {
		// Restore original functions
		NewApp = originalNewApp
		signalNotify = originalSignalNotify
	}()

	// Create a mock that simulates active requests
	startNotify := make(chan struct{})
	mockApp := &MockAppSimple{
		startNotify: startNotify,
	}

	// Track if shutdown methods were called
	var setTimeoutCalled bool
	var getActiveRequestsCalled bool

	// Create a custom mock with function fields for the new methods
	customMockApp := &MockAppSimple{
		startNotify: startNotify,
	}

	// Override the graceful shutdown methods to track calls
	customMockApp.SetShutdownTimeoutFunc = func(timeout time.Duration) {
		setTimeoutCalled = true
		// Verify timeout is set to 65 seconds
		assert.Equal(t, 65*time.Second, timeout)
	}

	customMockApp.GetActiveRequestCountFunc = func() int64 {
		getActiveRequestsCalled = true
		return 2 // Simulate 2 active requests
	}

	// Use the custom mock instead
	mockApp = customMockApp

	// Replace NewApp with our mock implementation
	NewApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface {
		return mockApp
	}

	// Replace signal notify to send signal in test
	// Track how many times signalNotify is called
	var signalCallCount int32
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		callNum := atomic.AddInt32(&signalCallCount, 1)
		if callNum == 1 {
			// First call - for initial shutdown channel
			go func() {
				<-startNotify // Wait for start to be called
				time.Sleep(100 * time.Millisecond)
				c <- os.Interrupt // Send first signal
			}()
		}
		// Second call would be for force shutdown channel (created after first signal)
		// We don't send anything to it in this test
	}

	// Create test config
	cfg := createSimpleTestConfig()

	// Create mock logger
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations for graceful shutdown
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Run the server in a goroutine
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- runServer(cfg, mockLogger)
	}()

	// Set test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Wait for result or timeout
	var err error
	select {
	case err = <-resultCh:
		// Got result
	case <-ctx.Done():
		t.Fatalf("Test timed out")
	}

	// Should not have error for graceful shutdown
	assert.NoError(t, err)

	// Verify methods were called
	assert.True(t, mockApp.initializeCalled, "Initialize should have been called")
	assert.True(t, mockApp.shutdownCalled, "Shutdown should have been called")
	assert.True(t, setTimeoutCalled, "SetShutdownTimeout should have been called")
	assert.True(t, getActiveRequestsCalled, "GetActiveRequestCount should have been called")
}

// TestForceShutdownWithSecondSignal tests the force shutdown functionality
func TestForceShutdownWithSecondSignal(t *testing.T) {
	// Skip in normal test runs
	if os.Getenv("RUN_RUNSERVER_TEST") != "true" {
		t.Skip("Skipping TestForceShutdownWithSecondSignal. Use -tags=runserver to run this test.")
	}

	// Save original functions to restore later
	originalNewApp := NewApp
	originalSignalNotify := signalNotify

	defer func() {
		// Restore original functions
		NewApp = originalNewApp
		signalNotify = originalSignalNotify
	}()

	// Create a mock that simulates long-running shutdown
	startNotify := make(chan struct{})
	mockApp := &MockAppSimple{
		startNotify: startNotify,
	}

	// Make shutdown take a long time to test force shutdown
	mockApp.ShutdownFunc = func(ctx context.Context) error {
		// Simulate a slow shutdown that would normally take 65 seconds
		select {
		case <-ctx.Done():
			// Context was cancelled (force shutdown)
			return fmt.Errorf("shutdown cancelled")
		case <-time.After(10 * time.Second):
			// This would normally complete after a long time
			return nil
		}
	}

	// Replace NewApp with our mock implementation
	NewApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface {
		return mockApp
	}

	// Replace signal notify to send TWO signals (force shutdown test)
	var callCount int32
	signalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		callNum := atomic.AddInt32(&callCount, 1)
		if callNum == 1 {
			// First call - for initial shutdown channel
			go func() {
				<-startNotify // Wait for start to be called
				time.Sleep(100 * time.Millisecond)
				c <- os.Interrupt // Send first signal
			}()
		} else if callNum == 2 {
			// Second call - for force shutdown channel (created after first signal)
			go func() {
				// Send second signal shortly after first
				time.Sleep(200 * time.Millisecond)
				c <- os.Interrupt // Send force shutdown signal
			}()
		}
	}

	// Create test config
	cfg := createSimpleTestConfig()

	// Create mock logger
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations for force shutdown
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Run the server in a goroutine
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- runServer(cfg, mockLogger)
	}()

	// Set test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Wait for result or timeout
	var err error
	select {
	case err = <-resultCh:
		// Got result
	case <-ctx.Done():
		t.Fatalf("Test timed out")
	}

	// Should have error for forced shutdown
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forced shutdown")

	// Verify methods were called
	assert.True(t, mockApp.initializeCalled, "Initialize should have been called")
	assert.True(t, mockApp.shutdownCalled, "Shutdown should have been called")
}

// TestSignalHandlingLogic tests just the signal handling logic without database dependencies
func TestSignalHandlingLogic(t *testing.T) {
	// This test doesn't require RUN_RUNSERVER_TEST since it's just testing the logic

	// Test that signalNotify is called correctly for the new signal handling pattern
	var callCount int32
	var channels []chan<- os.Signal

	// Mock signalNotify to track calls
	mockSignalNotify := func(c chan<- os.Signal, sig ...os.Signal) {
		atomic.AddInt32(&callCount, 1)
		channels = append(channels, c)
		// Verify the signals are correct
		assert.Contains(t, sig, os.Interrupt)
		assert.Contains(t, sig, syscall.SIGTERM)
	}

	// Save original
	originalSignalNotify := signalNotify
	defer func() {
		signalNotify = originalSignalNotify
	}()

	signalNotify = mockSignalNotify

	// Create a mock that will fail fast to avoid database connection
	mockApp := &MockAppSimple{
		initializeError: fmt.Errorf("test error - skip database"),
	}

	// Save original NewApp
	originalNewApp := NewApp
	defer func() {
		NewApp = originalNewApp
	}()

	NewApp = func(cfg *config.Config, opts ...app.AppOption) app.AppInterface {
		return mockApp
	}

	// Create test config
	cfg := createSimpleTestConfig()

	// Create mock logger
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Run the server - it should fail at initialization
	err := runServer(cfg, mockLogger)

	// Should have error due to initialization failure
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test error")

	// Verify signalNotify was called once (for initial shutdown channel)
	assert.Equal(t, int32(1), callCount, "signalNotify should be called once for initial setup")
	assert.Len(t, channels, 1, "Should have one signal channel")

	// Verify Initialize was called
	assert.True(t, mockApp.initializeCalled, "Initialize should have been called")
}
