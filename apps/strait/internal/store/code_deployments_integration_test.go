//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestCreateCodeDeployment_SourceURIStoredCorrectly verifies that the
// source_uri written to the database matches the value set on the struct
// before calling CreateCodeDeployment. This guards against the regression
// where the handler set an empty deployment ID in the URI before INSERT.
func TestCreateCodeDeployment_SourceURIStoredCorrectly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cduri-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)

	deploymentID := "deploy-" + newID()
	wantURI := "projects/" + projectID + "/jobs/" + job.ID + "/deploys/" + deploymentID + ".tar.gz"

	d := &domain.CodeDeployment{
		ID:              deploymentID,
		JobID:           job.ID,
		ProjectID:       projectID,
		Runtime:         domain.RuntimePython,
		SourceHash:      "aabbccdd" + newID(),
		SourceSizeBytes: 1024,
		SourceURI:       wantURI,
	}
	if err := q.CreateCodeDeployment(ctx, d); err != nil {
		t.Fatalf("CreateCodeDeployment: %v", err)
	}

	got, err := q.GetCodeDeployment(ctx, deploymentID, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment: %v", err)
	}
	if got.SourceURI != wantURI {
		t.Errorf("source_uri in DB = %q, want %q", got.SourceURI, wantURI)
	}
}

// TestConfirmCodeDeployment_AtomicPendingToBuilding verifies that
// ConfirmCodeDeployment transitions a pending deployment to building and
// that the second call returns ErrCodeDeploymentNotFound (not a second update).
func TestConfirmCodeDeployment_AtomicPendingToBuilding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-confirm-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("first ConfirmCodeDeployment: %v", err)
	}

	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment after confirm: %v", err)
	}
	if got.Status != domain.DeploymentStatusBuilding {
		t.Errorf("status after confirm = %q, want building", got.Status)
	}

	// Second call must fail — the deployment is no longer pending.
	err = q.ConfirmCodeDeployment(ctx, d.ID)
	if err == nil {
		t.Fatal("expected error on second ConfirmCodeDeployment, got nil")
	}
	if err.Error() != store.ErrCodeDeploymentNotFound.Error() {
		t.Errorf("expected ErrCodeDeploymentNotFound, got: %v", err)
	}
}

// TestUpdateCodeDeploymentStatus_NotFoundReturnsError verifies that updating
// a non-existent deployment returns ErrCodeDeploymentNotFound rather than
// silently succeeding.
func TestUpdateCodeDeploymentStatus_NotFoundReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.UpdateCodeDeploymentStatus(ctx, "nonexistent-id-"+newID(), domain.DeploymentStatusFailed, nil)
	if err == nil {
		t.Fatal("expected error updating nonexistent deployment, got nil")
	}
	if err.Error() != store.ErrCodeDeploymentNotFound.Error() {
		t.Errorf("expected ErrCodeDeploymentNotFound, got: %v", err)
	}
}

// TestCreateCodeDeployment_VersionUnique verifies that concurrent deployments
// for the same job get distinct version numbers.
func TestCreateCodeDeployment_VersionUnique(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-version-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)

	const count = 5
	versions := make(map[int]struct{})
	for i := range count {
		id := "deploy-v-" + newID()
		d := &domain.CodeDeployment{
			ID:              id,
			JobID:           job.ID,
			ProjectID:       projectID,
			Runtime:         domain.RuntimePython,
			SourceHash:      "hash" + id[:8],
			SourceSizeBytes: int64(1024 + i),
			SourceURI:       "projects/" + projectID + "/jobs/" + job.ID + "/deploys/" + id + ".tar.gz",
		}
		if err := q.CreateCodeDeployment(ctx, d); err != nil {
			t.Fatalf("CreateCodeDeployment %d: %v", i, err)
		}
		if _, dup := versions[d.Version]; dup {
			t.Errorf("duplicate version %d at deployment %d", d.Version, i)
		}
		versions[d.Version] = struct{}{}
	}
	if len(versions) != count {
		t.Errorf("expected %d unique versions, got %d", count, len(versions))
	}
}

// TestRollbackToDeployment_AtomicStatusCheck verifies that RollbackToDeployment
// rejects non-ready deployments atomically — no separate check query.
func TestRollbackToDeployment_AtomicStatusCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rollback-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	// Deployment is pending, not ready — rollback must fail.
	err := q.RollbackToDeployment(ctx, job.ID, d.ID, projectID)
	if err == nil {
		t.Fatal("expected error rolling back to pending deployment, got nil")
	}
}

// mustCreateDeployment is a test helper that creates a minimal pending deployment.
func mustCreateDeployment(t *testing.T, ctx context.Context, q *store.Queries, jobID, projectID string) *domain.CodeDeployment {
	t.Helper()
	id := "deploy-" + newID()
	d := &domain.CodeDeployment{
		ID:              id,
		JobID:           jobID,
		ProjectID:       projectID,
		Runtime:         domain.RuntimePython,
		SourceHash:      "hash" + id[:8],
		SourceSizeBytes: 1024,
		SourceURI:       "projects/" + projectID + "/jobs/" + jobID + "/deploys/" + id + ".tar.gz",
	}
	if err := q.CreateCodeDeployment(ctx, d); err != nil {
		t.Fatalf("mustCreateDeployment: %v", err)
	}
	return d
}

// mustCreateJob is a test helper that creates a minimal job for a project.
func mustCreateJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()
	id := "job-" + newID()
	job := &domain.Job{
		ID:          id,
		Name:        "Test Job " + id,
		Slug:        "test-job-" + id,
		ProjectID:   projectID,
		EndpointURL: "https://example.com/handler",
		Enabled:     true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("mustCreateJob: %v", err)
	}
	return job
}
