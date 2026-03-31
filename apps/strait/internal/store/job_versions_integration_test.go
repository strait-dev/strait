//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateJobVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-create")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		VersionID:   "vid-1",
		Name:        job.Name,
		Slug:        job.Slug,
		Description: "first version",
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}
	if v.CreatedAt.IsZero() {
		t.Fatal("CreateJobVersion() did not set CreatedAt")
	}
}

func TestGetJobVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-get")

	v := &domain.JobVersion{
		ID:            newID(),
		JobID:         job.ID,
		Version:       1,
		Name:          job.Name,
		Slug:          job.Slug,
		EndpointURL:   "https://example.com/v1",
		MaxAttempts:   3,
		TimeoutSecs:   30,
		Tags:          map[string]string{"team": "core"},
		PayloadSchema: json.RawMessage(`{"type":"object"}`),
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}

	got, err := q.GetJobVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobVersion() error = %v", err)
	}
	if got.ID != v.ID {
		t.Fatalf("GetJobVersion() id = %q, want %q", got.ID, v.ID)
	}
	if got.Tags["team"] != "core" {
		t.Fatalf("GetJobVersion() tags = %v, want team=core", got.Tags)
	}

	// Not found.
	_, err = q.GetJobVersion(ctx, job.ID, 99)
	if err == nil {
		t.Fatal("GetJobVersion(notfound) expected error, got nil")
	}
}

func TestListJobVersionsByJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-list")

	for i := 1; i <= 3; i++ {
		v := &domain.JobVersion{
			ID:          newID(),
			JobID:       job.ID,
			Version:     i,
			Name:        job.Name,
			Slug:        job.Slug,
			EndpointURL: "https://example.com/v",
			MaxAttempts: 3,
			TimeoutSecs: 30,
		}
		if err := q.CreateJobVersion(ctx, v); err != nil {
			t.Fatalf("CreateJobVersion(%d) error = %v", i, err)
		}
	}

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob() error = %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("ListJobVersionsByJob() len = %d, want 3", len(versions))
	}
	// Should be ordered version DESC.
	if versions[0].Version != 3 || versions[2].Version != 1 {
		t.Fatalf("ListJobVersionsByJob() version order = [%d,%d,%d], want [3,2,1]",
			versions[0].Version, versions[1].Version, versions[2].Version)
	}

	// Empty.
	empty, err := q.ListJobVersionsByJob(ctx, newID(), 10, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListJobVersionsByJob(empty) len = %d, want 0", len(empty))
	}
}

func TestGetJobVersionByVersionID_LookupByNanoid(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-by-vid")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		VersionID:   "nanoid-abc-123",
		Name:        job.Name,
		Slug:        job.Slug,
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}

	got, err := q.GetJobVersionByVersionID(ctx, "nanoid-abc-123")
	if err != nil {
		t.Fatalf("GetJobVersionByVersionID() error = %v", err)
	}
	if got.ID != v.ID {
		t.Fatalf("GetJobVersionByVersionID() id = %q, want %q", got.ID, v.ID)
	}

	// Not found.
	_, err = q.GetJobVersionByVersionID(ctx, "nonexistent")
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJobVersionByVersionID(notfound) error = %v, want ErrJobNotFound", err)
	}
}

func TestGetJobAtVersion_SnapshotAndFallback(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-at-version")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		Name:        "versioned-name",
		Slug:        job.Slug,
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}

	got, err := q.GetJobAtVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobAtVersion() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetJobAtVersion() returned nil")
	}
	if got.Name != "versioned-name" {
		t.Fatalf("GetJobAtVersion() name = %q, want %q", got.Name, "versioned-name")
	}

	// Fallback for a version that does not exist should return the live job.
	fallback, err := q.GetJobAtVersion(ctx, job.ID, 999)
	if err != nil {
		t.Fatalf("GetJobAtVersion(fallback) error = %v", err)
	}
	if fallback.ID != job.ID {
		t.Fatalf("GetJobAtVersion(fallback) id = %q, want %q", fallback.ID, job.ID)
	}
}
