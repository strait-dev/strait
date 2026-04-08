package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3StoreConfig holds configuration for an S3-compatible object store.
// Works with Cloudflare R2 (cloud edition), AWS S3, and MinIO (community).
type S3StoreConfig struct {
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	// Endpoint is the service URL. Leave empty for AWS S3.
	// For Cloudflare R2: "https://{account_id}.r2.cloudflarestorage.com"
	// For MinIO: "http://minio:9000"
	Endpoint string
	// ForcePathStyle enables path-style addressing required by MinIO.
	// R2 and AWS S3 use virtual-hosted-style by default.
	ForcePathStyle bool
}

// S3Store is an ObjectStore backed by any S3-compatible service.
type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

// NewS3Store creates a new S3Store from the provided config.
func NewS3Store(cfg S3StoreConfig) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("objectstore: bucket is required")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("objectstore: access key and secret key are required")
	}

	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")

	region := cfg.Region
	if region == "" {
		region = "auto"
	}

	optFns := []func(*s3.Options){
		func(o *s3.Options) {
			o.Credentials = creds
			o.Region = region
			o.UsePathStyle = cfg.ForcePathStyle
		},
	}
	if cfg.Endpoint != "" {
		optFns = append(optFns, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	client := s3.New(s3.Options{}, optFns...)

	return &S3Store{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.Bucket,
	}, nil
}

// PresignUpload generates a time-limited PUT presigned URL.
// contentLength is included in the signed request so S3/R2 rejects any upload
// whose Content-Length header does not match, preventing clients from uploading
// more data than they declared in source_size_bytes.
func (s *S3Store) PresignUpload(ctx context.Context, key string, ttl time.Duration, contentLength int64) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentLength: aws.Int64(contentLength),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("objectstore: presign upload: %w", err)
	}
	return req.URL, nil
}

// HeadObject checks whether an object exists and returns its content length.
func (s *S3Store) HeadObject(ctx context.Context, key string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		var notFound *s3types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &notFound) {
			return 0, ErrObjectNotFound
		}
		return 0, fmt.Errorf("objectstore: head object: %w", err)
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return size, nil
}

// GetObject returns a streaming reader for the object at key.
func (s *S3Store) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("objectstore: get object: %w", err)
	}
	return out.Body, nil
}

// PutObject uploads r to the store under key.
func (s *S3Store) PutObject(ctx context.Context, key string, r io.Reader, size int64) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("objectstore: put object: %w", err)
	}
	return nil
}

// DeleteObject removes the object at key. Deleting a non-existent key is a no-op.
func (s *S3Store) DeleteObject(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("objectstore: delete object: %w", err)
	}
	return nil
}
