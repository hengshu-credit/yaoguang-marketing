package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint       string
	Bucket         string
	Region         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
}

type S3Store struct {
	client *minio.Client
	bucket string
}

func NewS3Store(config S3Config) (*S3Store, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, errors.New("object store endpoint must be a valid http or https URL")
	}
	if endpoint.Path != "" && endpoint.Path != "/" || endpoint.RawQuery != "" || endpoint.User != nil {
		return nil, errors.New("object store endpoint must not contain path, query, or user info")
	}
	if strings.TrimSpace(config.Bucket) == "" || strings.ContainsAny(config.Bucket, `/\`) {
		return nil, errors.New("object store bucket is invalid")
	}
	if config.AccessKey == "" || config.SecretKey == "" {
		return nil, errors.New("object store access and secret keys are required")
	}
	lookup := minio.BucketLookupAuto
	if config.ForcePathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: endpoint.Scheme == "https", Region: config.Region, BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("create object store client: %w", err)
	}
	return &S3Store{client: client, bucket: config.Bucket}, nil
}

func (s *S3Store) Put(
	ctx context.Context,
	key string,
	body io.Reader,
	size int64,
	contentType string,
) (ObjectInfo, error) {
	if err := validateObjectKey(key); err != nil {
		return ObjectInfo{}, err
	}
	if body == nil || size < 0 {
		return ObjectInfo{}, errors.New("object body and non-negative size are required")
	}
	options := minio.PutObjectOptions{ContentType: contentType}
	// If-None-Match makes immutable asset versions atomic even with concurrent
	// writers; no template metadata is copied into object tags.
	options.SetMatchETagExcept("*")
	upload, err := s.client.PutObject(ctx, s.bucket, key, body, size, options)
	if err != nil {
		return ObjectInfo{}, mapS3Error("put", key, err)
	}
	return ObjectInfo{
		Key: key, Size: upload.Size, ETag: upload.ETag,
		ContentType: contentType, LastModified: time.Now().UTC(),
	}, nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, ObjectInfo{}, err
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, mapS3Error("get", key, err)
	}
	stat, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, ObjectInfo{}, mapS3Error("get", key, err)
	}
	return object, ObjectInfo{
		Key: key, Size: stat.Size, ETag: stat.ETag,
		ContentType: stat.ContentType, LastModified: stat.LastModified.UTC(),
	}, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := validateObjectKey(key); err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return mapS3Error("delete", key, err)
	}
	return nil
}

func (s *S3Store) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > 7*24*time.Hour {
		return "", errors.New("presigned URL ttl must be positive and no more than seven days")
	}
	presigned, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", mapS3Error("presign", key, err)
	}
	return presigned.String(), nil
}

func mapS3Error(operation, key string, err error) error {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NotFound", "NoSuchBucket":
		return &NotFoundError{Key: key}
	case "PreconditionFailed", "ConditionalRequestConflict":
		return &ConflictError{Key: key}
	default:
		return &UnavailableError{Operation: operation, Key: key, Err: err}
	}
}
