// Package objectstore provides a storage abstraction for code deployment tarballs.
//
// Cloud edition uses Cloudflare R2 (S3-compatible). Community edition can plug
// in any S3-compatible store (MinIO, AWS S3, etc.) by pointing OBJECT_STORE_ENDPOINT
// at their endpoint.
package objectstore

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrObjectNotFound is returned when the requested object does not exist.
var ErrObjectNotFound = errors.New("object not found")

// ObjectStore abstracts blob storage for deployment tarballs and build artifacts.
//
// Implementations must be safe for concurrent use.
type ObjectStore interface {
	// PresignUpload generates a time-limited URL that clients use to PUT an object
	// directly to the store without routing through the Strait API. The key must
	// be unique per deployment (e.g. "projects/{pid}/jobs/{jid}/deploys/{did}.tar.gz").
	PresignUpload(ctx context.Context, key string, ttl time.Duration) (url string, err error)

	// HeadObject checks whether an object exists and returns its size in bytes.
	// Returns ErrObjectNotFound if the key does not exist.
	HeadObject(ctx context.Context, key string) (sizeBytes int64, err error)

	// GetObject returns a streaming reader for the object at key.
	// The caller is responsible for closing the returned reader.
	// Returns ErrObjectNotFound if the key does not exist.
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)

	// PutObject writes r to the store under key. size must be the exact content
	// length; use -1 only for streaming writes where length is unknown.
	PutObject(ctx context.Context, key string, r io.Reader, size int64) error

	// DeleteObject removes the object at key. Deleting a non-existent key is not
	// an error.
	DeleteObject(ctx context.Context, key string) error
}

// DeploymentKey returns the canonical object store key for a deployment tarball.
func DeploymentKey(projectID, jobID, deploymentID string) string {
	return "projects/" + projectID + "/jobs/" + jobID + "/deploys/" + deploymentID + ".tar.gz"
}
