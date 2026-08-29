// Package objectstore provides the small S3-compatible boundary used for
// binary marketing assets. Business metadata remains in PostgreSQL.
package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
}

type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (ObjectInfo, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type UnavailableError struct {
	Operation string
	Key       string
	Err       error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("object store %s unavailable for %q: %v", e.Operation, e.Key, e.Err)
}
func (e *UnavailableError) Unwrap() error { return e.Err }

type NotFoundError struct{ Key string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("object %q not found", e.Key) }

type ConflictError struct{ Key string }

func (e *ConflictError) Error() string { return fmt.Sprintf("object %q already exists", e.Key) }

func WorkspaceObjectKey(workspaceID, assetID string, version int, filename string) (string, error) {
	if version <= 0 {
		return "", fmt.Errorf("asset version must be positive")
	}
	for name, value := range map[string]string{
		"workspace id": workspaceID, "asset id": assetID, "filename": filename,
	} {
		if err := validateObjectComponent(value); err != nil {
			return "", fmt.Errorf("invalid %s: %w", name, err)
		}
	}
	return fmt.Sprintf(
		"workspaces/%s/assets/%s/v%d/%s",
		url.PathEscape(workspaceID), url.PathEscape(assetID), version, url.PathEscape(filename),
	), nil
}

func validateObjectComponent(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value is required")
	}
	if value == "." || value == ".." || strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return fmt.Errorf("path separators and traversal components are not allowed")
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("control characters are not allowed")
		}
	}
	return nil
}

func validateObjectKey(key string) error {
	if strings.TrimSpace(key) == "" || strings.HasPrefix(key, "/") || strings.Contains(key, `\`) {
		return fmt.Errorf("invalid object key")
	}
	for _, component := range strings.Split(key, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("invalid object key path component")
		}
	}
	for _, character := range key {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("object key contains a control character")
		}
	}
	return nil
}
