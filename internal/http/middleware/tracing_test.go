package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opencensus.io/trace"
)

func TestTracingMiddleware(t *testing.T) {
	// Create a test handler that checks for span attributes
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if trace context exists
		span := trace.FromContext(r.Context())
		if span == nil {
			t.Error("Expected trace span to be in context")
		}

		// Return a 200 status code
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with tracing middleware
	handler := TracingMiddleware(testHandler)

	// Create a test request
	req, err := http.NewRequest("GET", "/test-path?param=value", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set test headers
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-request-id")
	req.Host = "test-host"

	// Create a response recorder
	recorder := httptest.NewRecorder()

	// Process the request through the middleware
	handler.ServeHTTP(recorder, req)

	// Check response status
	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Additional checks can be added, but span attributes are hard to test
	// without dependency injection for the tracer
}

// Test with existing span in context
func TestTracingMiddleware_WithExistingSpan(t *testing.T) {
	// Create a test handler that checks for span attributes
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if trace context exists and has attributes
		span := trace.FromContext(r.Context())
		if span == nil {
			t.Error("Expected trace span to be in context")
		}

		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with tracing middleware
	handler := TracingMiddleware(testHandler)

	// Create a test request with an existing trace context
	req, err := http.NewRequest("GET", "/test-path", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set test headers
	req.Header.Set("User-Agent", "test-agent")
	req.Host = "test-host"

	// Add a trace context
	ctx, span := trace.StartSpan(req.Context(), "parent-span")
	defer span.End()
	req = req.WithContext(ctx)

	// Create a response recorder
	recorder := httptest.NewRecorder()

	// Process the request through the middleware
	handler.ServeHTTP(recorder, req)

	// Check response status
	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

// Test with error response status code
func TestTracingMiddleware_WithErrorStatus(t *testing.T) {
	// Create a test handler that returns an error status
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a 500 status code
		w.WriteHeader(http.StatusInternalServerError)
	})

	// Wrap the test handler with tracing middleware
	handler := TracingMiddleware(testHandler)

	// Create a test request
	req, err := http.NewRequest("GET", "/test-path", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a response recorder
	recorder := httptest.NewRecorder()

	// Process the request through the middleware
	handler.ServeHTTP(recorder, req)

	// Check response status
	if status := recorder.Code; status != http.StatusInternalServerError {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}

	// The status code should have been captured in the span, but we can't verify
	// that easily in a test without mocking the tracer
}

// Test all the response writer methods are correctly implemented
func TestTraceResponseWriter(t *testing.T) {
	// Create a test response writer
	recorder := httptest.NewRecorder()

	// Create a basic context with a span
	ctx, span := trace.StartSpan(context.Background(), "test-span")
	defer span.End()

	w := &traceResponseWriter{ResponseWriter: recorder, ctx: ctx}

	// Set a status code
	w.WriteHeader(http.StatusOK)

	// Check the status code was recorded
	if w.statusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.statusCode)
	}

	// Test writing response body
	_, err := w.Write([]byte("test"))
	if err != nil {
		t.Errorf("Error writing response: %v", err)
	}

	// Verify body was written to underlying response writer
	if body := recorder.Body.String(); body != "test" {
		t.Errorf("Expected body 'test', got '%s'", body)
	}
}
