package domain

import (
	"context"
	"errors"
)

type executionMediaArchiveKey struct{}
type ExecutionMediaArchive func(context.Context, string, string, string, any) error

func WithExecutionMediaArchive(ctx context.Context, archive ExecutionMediaArchive) context.Context {
	return context.WithValue(ctx, executionMediaArchiveKey{}, archive)
}
func CaptureExecutionMedia(ctx context.Context, workspaceID, executionID, deliveryID string, content any) error {
	if executionID == "" {
		return nil
	}
	archive, ok := ctx.Value(executionMediaArchiveKey{}).(ExecutionMediaArchive)
	if !ok {
		return errors.New("execution media archival is unavailable")
	}
	return archive(ctx, workspaceID, executionID, deliveryID, content)
}
