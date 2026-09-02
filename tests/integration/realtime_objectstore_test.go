package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/objectstore"
)

func TestRealtimeObjectStore(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is not configured")
	}
	bucket := os.Getenv("TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "marketing-assets"
	}
	store, err := objectstore.NewS3Store(objectstore.S3Config{
		Endpoint: endpoint, Bucket: bucket, Region: os.Getenv("TEST_S3_REGION"),
		AccessKey: os.Getenv("TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TEST_S3_SECRET_KEY"),
		ForcePathStyle: true,
	})
	require.NoError(t, err)

	key, err := objectstore.WorkspaceObjectKey("integration-"+uuid.NewString(), "asset-1", 1, "hello.txt")
	require.NoError(t, err)
	body := []byte("hello realtime object storage")
	ctx := context.Background()
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })

	info, err := store.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/plain")
	require.NoError(t, err)
	assert.Equal(t, int64(len(body)), info.Size)

	_, err = store.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/plain")
	var conflict *objectstore.ConflictError
	require.True(t, errors.As(err, &conflict))

	reader, loadedInfo, err := store.Get(ctx, key)
	require.NoError(t, err)
	loaded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, body, loaded)
	assert.Equal(t, "text/plain", loadedInfo.ContentType)

	presigned, err := store.PresignGet(ctx, key, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, presigned)

	require.NoError(t, store.Delete(ctx, key))
	_, _, err = store.Get(ctx, key)
	var notFound *objectstore.NotFoundError
	assert.True(t, errors.As(err, &notFound))
}
