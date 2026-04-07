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

// TestClaimBuildingDeployment_TwoWorkersNoDuplicate verifies that two concurrent
// workers each claim a different deployment and neither sees the other's row.
func TestClaimBuildingDeployment_TwoWorkersNoDuplicate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-claim-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)

	d1 := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	d2 := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	// Transition both to building.
	if err := q.ConfirmCodeDeployment(ctx, d1.ID); err != nil {
		t.Fatalf("confirm d1: %v", err)
	}
	if err := q.ConfirmCodeDeployment(ctx, d2.ID); err != nil {
		t.Fatalf("confirm d2: %v", err)
	}

	claimed1, err := q.ClaimBuildingDeployment(ctx, "worker-A")
	if err != nil {
		t.Fatalf("claim by worker-A: %v", err)
	}
	if claimed1 == nil {
		t.Fatal("worker-A expected to claim a deployment, got nil")
	}

	claimed2, err := q.ClaimBuildingDeployment(ctx, "worker-B")
	if err != nil {
		t.Fatalf("claim by worker-B: %v", err)
	}
	if claimed2 == nil {
		t.Fatal("worker-B expected to claim a deployment, got nil")
	}

	if claimed1.ID == claimed2.ID {
		t.Errorf("two workers claimed the same deployment %s", claimed1.ID)
	}

	// A third claim should return nil — queue is empty.
	none, err := q.ClaimBuildingDeployment(ctx, "worker-C")
	if err != nil {
		t.Fatalf("claim by worker-C: %v", err)
	}
	if none != nil {
		t.Errorf("worker-C expected nil (queue empty), got deployment %s", none.ID)
	}
}

// TestClaimBuildingDeployment_ReturnsNilWhenNoneAvailable verifies that claiming
// against an empty queue returns nil without error.
func TestClaimBuildingDeployment_ReturnsNilWhenNoneAvailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.ClaimBuildingDeployment(ctx, "worker-X")
	if err != nil {
		t.Fatalf("ClaimBuildingDeployment on empty queue: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got deployment %s", got.ID)
	}
}

// TestReleaseStaleClaimedDeployments_ReclearsExpired verifies that deployments
// with a claim older than the cutoff are released so they can be reclaimed.
func TestReleaseStaleClaimedDeployments_ReclearsExpired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stale-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Claim it.
	claimed, err := q.ClaimBuildingDeployment(ctx, "worker-stale")
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v, %v", claimed, err)
	}

	// Release with a very short olderThan so everything qualifies as stale.
	released, err := q.ReleaseStaleClaimedDeployments(ctx, -1*time.Second)
	if err != nil {
		t.Fatalf("ReleaseStaleClaimedDeployments: %v", err)
	}
	if released == 0 {
		t.Error("expected at least one stale claim to be released")
	}

	// Now it should be claimable again.
	reclaimed, err := q.ClaimBuildingDeployment(ctx, "worker-recovery")
	if err != nil {
		t.Fatalf("reclaim after release: %v", err)
	}
	if reclaimed == nil {
		t.Fatal("expected deployment to be reclaimable after stale release, got nil")
	}
	if reclaimed.ID != d.ID {
		t.Errorf("expected reclaimed deployment %s, got %s", d.ID, reclaimed.ID)
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
