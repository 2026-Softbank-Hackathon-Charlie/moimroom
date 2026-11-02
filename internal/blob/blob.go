// Package blob stores space photos in S3-compatible object storage.
// The Store interface keeps handlers testable without a real bucket.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/2026-Softbank-Hackathon-Charlie/moimroom/internal/config"
)

var ErrNotFound = errors.New("object not found")

// Object is an open object body. The caller closes Body.
type Object struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
}

type Store interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (Object, error)
}

// S3 implements Store with minio-go, which speaks to AWS S3, GCS
// (interoperability mode with HMAC keys) and MinIO alike.
type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(c config.S3) (*S3, error) {
	lookup := minio.BucketLookupAuto
	switch c.PathStyle {
	case "true":
		lookup = minio.BucketLookupPath
	case "false":
		lookup = minio.BucketLookupDNS
	}
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure:       c.Secure,
		Region:       c.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return &S3{client: client, bucket: c.Bucket}, nil
}

// CheckBucket reports a missing bucket or bad credentials at startup.
func (s *S3) CheckBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3) Get(ctx context.Context, key string) (Object, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return Object{}, err
	}
	// GetObject is lazy; Stat surfaces NoSuchKey before we write headers.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return Object{}, ErrNotFound
		}
		return Object{}, err
	}
	return Object{Body: obj, Size: info.Size, ContentType: info.ContentType}, nil
}
