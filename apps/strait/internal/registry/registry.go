// Package registry provides a container registry abstraction for code-first deployments.
//
// Cloud edition uses AWS ECR. Community edition can use any Docker Registry API v2
// compatible registry (Harbor, GHCR, Docker Hub, self-hosted) via GenericRegistry.
package registry

import (
	"context"
	"errors"
	"time"
)

// ErrRepositoryNotFound is returned when the named repository does not exist.
var ErrRepositoryNotFound = errors.New("registry: repository not found")

// ErrImageNotFound is returned when the requested image tag or digest does not exist.
var ErrImageNotFound = errors.New("registry: image not found")

// ContainerRegistry abstracts the container registry used to store built images.
//
// Implementations must be safe for concurrent use.
type ContainerRegistry interface {
	// EnsureRepository creates the repository if it does not exist and returns its
	// push URI (e.g. "123456789.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/my-job").
	// Idempotent: calling again on an existing repository returns its URI unchanged.
	EnsureRepository(ctx context.Context, name string) (repositoryURI string, err error)

	// GetAuthToken returns a base64-encoded "user:password" token suitable for use
	// as a Docker registry auth credential. The token is valid until expiresAt.
	// BuildKit uses this to authenticate push/pull operations during builds.
	GetAuthToken(ctx context.Context) (token string, expiresAt time.Time, err error)

	// GetImageDigest fetches the content-addressable digest (sha256:...) of the
	// image at repositoryURI:tag. Returns ErrImageNotFound if the tag does not exist.
	GetImageDigest(ctx context.Context, repositoryURI, tag string) (digest string, err error)
}

// JobRepositoryName returns the canonical registry repository name for a job's images.
// Consistent naming ensures each job has exactly one repository and old images can
// be pruned by ECR lifecycle policy without touching other jobs.
func JobRepositoryName(prefix, projectID, jobID string) string {
	if prefix == "" {
		prefix = "strait-jobs"
	}
	return prefix + "/" + projectID + "/" + jobID
}
