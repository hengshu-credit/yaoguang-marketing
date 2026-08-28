package tracing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opencensus.io/trace"
)

// StartServiceSpan starts a new span for a service method
func StartServiceSpan(ctx context.Context, serviceName, methodName string) (context.Context, *trace.Span) {
	return trace.StartSpan(ctx, fmt.Sprintf("%s.%s", serviceName, methodName))
}

// EndSpan ends a span and records any error
func EndSpan(span *trace.Span, err error) {
	if err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnknown,
			Message: err.Error(),
		})
	}
	span.End()
}

// TraceMethod is a helper to trace a service method with automatic span ending
func TraceMethod(ctx context.Context, serviceName, methodName string, f func(context.Context) error) error {
	ctx, span := StartServiceSpan(ctx, serviceName, methodName)
	defer span.End()

	err := f(ctx)
	if err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnknown,
			Message: err.Error(),
		})
	}

	return err
}

// TraceMethodWithResult is a helper to trace a service method that returns a result
func TraceMethodWithResult[T any](
	ctx context.Context,
	serviceName,
	methodName string,
	f func(context.Context) (T, error),
) (T, error) {
	ctx, span := StartServiceSpan(ctx, serviceName, methodName)
	defer span.End()

	result, err := f(ctx)
	if err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnknown,
			Message: err.Error(),
		})
	}

	return result, err
}

// AddAttribute adds an attribute to the current span
func AddAttribute(ctx context.Context, key string, value interface{}) {
	span := trace.FromContext(ctx)
	if span == nil {
		return
	}

	switch v := value.(type) {
	case string:
		span.AddAttributes(trace.StringAttribute(key, v))
	case int64:
		span.AddAttributes(trace.Int64Attribute(key, v))
	case int32:
		span.AddAttributes(trace.Int64Attribute(key, int64(v)))
	case int:
		span.AddAttributes(trace.Int64Attribute(key, int64(v)))
	case bool:
		span.AddAttributes(trace.BoolAttribute(key, v))
	default:
		span.AddAttributes(trace.StringAttribute(key, fmt.Sprintf("%v", v)))
	}
}

// MarkSpanError marks the current span as failed with the given error
func MarkSpanError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	span := trace.FromContext(ctx)
	if span == nil {
		return
	}

	span.SetStatus(trace.Status{
		Code:    trace.StatusCodeUnknown,
		Message: err.Error(),
	})
}

// WrapHTTPClient wraps an http.Client with OpenCensus tracing
func WrapHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	transport := GetHTTPOptions()
	transport.Base = client.Transport

	return &http.Client{
		Transport:     &transport,
		Timeout:       client.Timeout,
		Jar:           client.Jar,
		CheckRedirect: client.CheckRedirect,
	}
}
