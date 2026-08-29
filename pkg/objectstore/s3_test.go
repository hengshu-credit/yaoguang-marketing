package objectstore

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceObjectKeyIsTenantAssetAndVersionScoped(t *testing.T) {
	key, err := WorkspaceObjectKey("workspace-1", "asset-2", 7, "hero image.png")
	require.NoError(t, err)
	assert.Equal(t, "workspaces/workspace-1/assets/asset-2/v7/hero%20image.png", key)

	other, err := WorkspaceObjectKey("workspace-2", "asset-2", 7, "hero image.png")
	require.NoError(t, err)
	assert.NotEqual(t, key, other)
}

func TestWorkspaceObjectKeyRejectsTraversalAndInvalidVersion(t *testing.T) {
	for _, test := range []struct {
		workspace string
		asset     string
		version   int
		filename  string
	}{
		{"../workspace", "asset", 1, "file.png"},
		{"workspace", "..", 1, "file.png"},
		{"workspace", "asset", 1, "../file.png"},
		{"workspace", "asset", 1, `folder\file.png`},
		{"workspace", "asset", 0, "file.png"},
	} {
		_, err := WorkspaceObjectKey(test.workspace, test.asset, test.version, test.filename)
		require.Error(t, err)
	}
}

func TestS3PresignUsesRequestedTTLWithoutLeakingSecret(t *testing.T) {
	store, err := NewS3Store(S3Config{
		Endpoint: "https://objects.example.com", Bucket: "assets", Region: "us-east-1",
		AccessKey: "access-key", SecretKey: "super-secret", ForcePathStyle: true,
	})
	require.NoError(t, err)

	presigned, err := store.PresignGet(context.Background(), "workspaces/ws/assets/a/v1/file.png", 15*time.Minute)
	require.NoError(t, err)
	parsed, err := url.Parse(presigned)
	require.NoError(t, err)
	assert.Equal(t, "900", parsed.Query().Get("X-Amz-Expires"))
	assert.False(t, strings.Contains(presigned, "super-secret"))
}

func TestS3PresignRejectsInvalidTTLAndKey(t *testing.T) {
	store, err := NewS3Store(S3Config{
		Endpoint: "http://127.0.0.1:9000", Bucket: "assets", Region: "us-east-1",
		AccessKey: "access-key", SecretKey: "super-secret", ForcePathStyle: true,
	})
	require.NoError(t, err)
	_, err = store.PresignGet(context.Background(), "../secret", time.Minute)
	require.Error(t, err)
	_, err = store.PresignGet(context.Background(), "workspaces/ws/assets/a/v1/file.png", 0)
	require.Error(t, err)
	_, err = store.PresignGet(context.Background(), "workspaces/ws/assets/a/v1/file.png", 8*24*time.Hour)
	require.Error(t, err)
}
