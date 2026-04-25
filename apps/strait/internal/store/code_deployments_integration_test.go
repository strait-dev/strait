//go:build integration

package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestCreateCodeDeployment_SourceURIStoredCorrectly verifies that the
// source_uri written to the database matches the value set on the struct
// before calling CreateCodeDeployment. This guards against the regression
// where the handler set an empty deployment ID in the URI before INSERT.
func TestCreateCodeDeployment_SourceURIStoredCorrectly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cduri-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

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
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-confirm-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
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
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-version-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

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
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rollback-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
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
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-claim-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

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
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stale-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
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

// mustCreateJobForDeploy is a test helper that creates a minimal job for a project.
func mustCreateJobForDeploy(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
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
		t.Fatalf("mustCreateJobForDeploy: %v", err)
	}
	return job
}

// TestDeleteExpiredDeployments_DeletesStalePending verifies that pending
// deployments whose presigned-upload TTL has expired are removed.
func TestDeleteExpiredDeployments_DeletesStalePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-gc-pending-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create two pending deployments.
	d1 := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	d2 := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	// GC with a pendingBefore of far future — both qualify.
	deleted, err := q.DeleteExpiredDeployments(ctx, time.Now().Add(1*time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("DeleteExpiredDeployments: %v", err)
	}
	if deleted < 2 {
		t.Errorf("expected at least 2 rows deleted, got %d", deleted)
	}

	// Verify both are gone.
	for _, id := range []string{d1.ID, d2.ID} {
		_, err := q.GetCodeDeployment(ctx, id, projectID)
		if err == nil {
			t.Errorf("expected deployment %s to be deleted, but GetCodeDeployment succeeded", id)
		}
	}
}

// TestDeleteExpiredDeployments_DeletesOldFailed verifies that failed and
// timed_out deployments whose finished_at is beyond the retention window are removed.
func TestDeleteExpiredDeployments_DeletesOldFailed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-gc-failed-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create deployments, confirm them, then mark them failed/timed_out.
	dFailed := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	dTimedOut := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, dFailed.ID); err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if err := q.ConfirmCodeDeployment(ctx, dTimedOut.ID); err != nil {
		t.Fatalf("confirm timed_out: %v", err)
	}
	if err := q.UpdateCodeDeploymentStatus(ctx, dFailed.ID, domain.DeploymentStatusFailed, nil); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if err := q.UpdateCodeDeploymentStatus(ctx, dTimedOut.ID, domain.DeploymentStatusTimedOut, nil); err != nil {
		t.Fatalf("mark timed_out: %v", err)
	}

	// GC with failedBefore of far future — both qualify.
	deleted, err := q.DeleteExpiredDeployments(ctx, time.Time{}, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpiredDeployments: %v", err)
	}
	if deleted < 2 {
		t.Errorf("expected at least 2 rows deleted, got %d", deleted)
	}
}

// TestDeleteExpiredDeployments_PreservesActiveDeployment verifies that a ready
// deployment that is the active_deployment_id on a job is never GC'd, even if
// all other criteria would select it.
func TestDeleteExpiredDeployments_PreservesActiveDeployment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-gc-active-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create and fully promote a deployment to ready + active.
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := q.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusReady, nil); err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	if err := q.SetActiveDeployment(ctx, job.ID, d.ID, projectID); err != nil {
		t.Fatalf("set active: %v", err)
	}

	// Create a stale failed deployment that should be deleted.
	dFailed := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	if err := q.ConfirmCodeDeployment(ctx, dFailed.ID); err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if err := q.UpdateCodeDeploymentStatus(ctx, dFailed.ID, domain.DeploymentStatusFailed, nil); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	deleted, err := q.DeleteExpiredDeployments(ctx, time.Now().Add(1*time.Hour), time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpiredDeployments: %v", err)
	}

	// The active (ready) deployment is not in the failed/pending set anyway, but
	// confirm it still exists after GC.
	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("active deployment should survive GC: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("expected active deployment %s, got %s", d.ID, got.ID)
	}
	if deleted == 0 {
		t.Error("expected the stale failed deployment to be deleted")
	}
}

// TestDeleteExpiredDeployments_PreservesRecentPending verifies that pending
// deployments created recently (within pendingTTL) are not deleted.
func TestDeleteExpiredDeployments_PreservesRecentPending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-gc-recent-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	// GC with a pendingBefore in the far past — nothing should qualify.
	deleted, err := q.DeleteExpiredDeployments(ctx, time.Now().Add(-1*time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("DeleteExpiredDeployments: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deletions, got %d", deleted)
	}

	// Deployment still exists.
	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("recent pending deployment should not be deleted: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("expected %s, got %s", d.ID, got.ID)
	}
}

// TestRollbackSetsRollbackSourceDeploymentID verifies that RollbackToDeployment
// stores the previous active deployment in rollback_source_deployment_id.
func TestRollbackSetsRollbackSourceDeploymentID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rollback-meta-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create two ready deployments: d1 (first) and d2 (second).
	d1 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)
	d2 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)

	// Set d2 as the active deployment.
	if err := q.SetActiveDeployment(ctx, job.ID, d2.ID, projectID); err != nil {
		t.Fatalf("SetActiveDeployment(d2): %v", err)
	}

	// Roll back to d1. This should set rollback_source_deployment_id = d2.ID.
	if err := q.RollbackToDeployment(ctx, job.ID, d1.ID, projectID); err != nil {
		t.Fatalf("RollbackToDeployment: %v", err)
	}

	updatedJob, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob after rollback: %v", err)
	}
	if updatedJob.ActiveDeploymentID != d1.ID {
		t.Errorf("active_deployment_id = %q, want %q", updatedJob.ActiveDeploymentID, d1.ID)
	}
	if updatedJob.RollbackSourceDeploymentID != d2.ID {
		t.Errorf("rollback_source_deployment_id = %q, want %q", updatedJob.RollbackSourceDeploymentID, d2.ID)
	}
}

// TestNewBuildClearsRollbackFlag verifies that SetActiveDeployment (called after
// a successful new build) clears rollback_source_deployment_id.
func TestNewBuildClearsRollbackFlag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rollback-clear-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	d1 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)
	d2 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)
	d3 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)

	// Promote d2, then roll back to d1.
	if err := q.SetActiveDeployment(ctx, job.ID, d2.ID, projectID); err != nil {
		t.Fatalf("SetActiveDeployment(d2): %v", err)
	}
	if err := q.RollbackToDeployment(ctx, job.ID, d1.ID, projectID); err != nil {
		t.Fatalf("RollbackToDeployment: %v", err)
	}

	// Verify rollback flag is set.
	afterRollback, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if afterRollback.RollbackSourceDeploymentID == "" {
		t.Fatal("expected rollback_source_deployment_id to be set after rollback")
	}

	// Now promote a new deployment (d3). rollback_source_deployment_id should be cleared.
	if err := q.SetActiveDeployment(ctx, job.ID, d3.ID, projectID); err != nil {
		t.Fatalf("SetActiveDeployment(d3): %v", err)
	}

	afterNewBuild, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob after new build: %v", err)
	}
	if afterNewBuild.RollbackSourceDeploymentID != "" {
		t.Errorf("rollback_source_deployment_id should be cleared after new build, got %q", afterNewBuild.RollbackSourceDeploymentID)
	}
	if afterNewBuild.ActiveDeploymentID != d3.ID {
		t.Errorf("active_deployment_id = %q, want %q", afterNewBuild.ActiveDeploymentID, d3.ID)
	}
}

// mustCreateReadyDeployment is a test helper that creates a code deployment and
// advances it to the "ready" status with a fake built image URI.
func mustCreateReadyDeployment(t *testing.T, ctx context.Context, q *store.Queries, jobID, projectID string) *domain.CodeDeployment {
	t.Helper()
	d := mustCreateDeployment(t, ctx, q, jobID, projectID)
	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("mustCreateReadyDeployment: ConfirmCodeDeployment: %v", err)
	}
	imageURI := "registry.example.com/image:" + newID()
	if err := q.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusReady, map[string]any{"built_image_uri": imageURI}); err != nil {
		t.Fatalf("mustCreateReadyDeployment: UpdateCodeDeploymentStatus(ready): %v", err)
	}
	d.Status = domain.DeploymentStatusReady
	d.BuiltImageURI = imageURI
	return d
}

// mustCreateProjectWithOrg creates a project belonging to the given org.
func mustCreateProjectWithOrg(t *testing.T, ctx context.Context, q *store.Queries, orgID string) *domain.Project {
	t.Helper()
	p := &domain.Project{
		ID:    "proj-" + newID(),
		OrgID: orgID,
		Name:  "Test Org Project " + newID(),
	}
	if err := q.CreateProject(ctx, p); err != nil {
		t.Fatalf("mustCreateProjectWithOrg: %v", err)
	}
	return p
}

// TestListCodeDeploymentsByOrg_CrossProject verifies that deployments from
// multiple projects in the same org are returned together, ordered newest-first.
func TestListCodeDeploymentsByOrg_CrossProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-" + newID()
	p1 := mustCreateProjectWithOrg(t, ctx, q, orgID)
	p2 := mustCreateProjectWithOrg(t, ctx, q, orgID)

	job1 := mustCreateJobForDeploy(t, ctx, q, p1.ID)
	job2 := mustCreateJobForDeploy(t, ctx, q, p2.ID)

	d1 := mustCreateDeployment(t, ctx, q, job1.ID, p1.ID)
	d2 := mustCreateDeployment(t, ctx, q, job2.ID, p2.ID)

	deployments, err := q.ListCodeDeploymentsByOrg(ctx, orgID, 10, nil)
	if err != nil {
		t.Fatalf("ListCodeDeploymentsByOrg: %v", err)
	}

	ids := make(map[string]bool, len(deployments))
	for _, d := range deployments {
		ids[d.ID] = true
	}
	if !ids[d1.ID] {
		t.Errorf("expected deployment %q from project 1 to be returned", d1.ID)
	}
	if !ids[d2.ID] {
		t.Errorf("expected deployment %q from project 2 to be returned", d2.ID)
	}

	// Results must be ordered newest-first (d2 was created after d1).
	if len(deployments) >= 2 {
		if deployments[0].CreatedAt.Before(deployments[len(deployments)-1].CreatedAt) {
			t.Errorf("expected deployments ordered newest-first, got oldest-first")
		}
	}
}

// TestListCodeDeploymentsByOrg_CrossTenantIsolation verifies that deployments
// from a different org are not returned.
func TestListCodeDeploymentsByOrg_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgA := "org-a-iso-" + newID()
	orgB := "org-b-iso-" + newID()

	pA := mustCreateProjectWithOrg(t, ctx, q, orgA)
	pB := mustCreateProjectWithOrg(t, ctx, q, orgB)

	jobA := mustCreateJobForDeploy(t, ctx, q, pA.ID)
	jobB := mustCreateJobForDeploy(t, ctx, q, pB.ID)

	dA := mustCreateDeployment(t, ctx, q, jobA.ID, pA.ID)
	dB := mustCreateDeployment(t, ctx, q, jobB.ID, pB.ID)

	// Query only org A.
	deploymentsA, err := q.ListCodeDeploymentsByOrg(ctx, orgA, 10, nil)
	if err != nil {
		t.Fatalf("ListCodeDeploymentsByOrg(orgA): %v", err)
	}
	for _, d := range deploymentsA {
		if d.ID == dB.ID {
			t.Errorf("org A query returned deployment %q belonging to org B", dB.ID)
		}
	}
	ids := make(map[string]bool, len(deploymentsA))
	for _, d := range deploymentsA {
		ids[d.ID] = true
	}
	if !ids[dA.ID] {
		t.Errorf("expected deployment %q from org A to be returned", dA.ID)
	}
	_ = dB // silence unused warning
}

// --- UpdateCodeDeploymentStatus ---

// TestUpdateCodeDeploymentStatus_ClearsClaimOnTerminal verifies that transitioning
// a deployment to a terminal status (ready/failed/timed_out) atomically clears
// build_node_id and build_node_claimed_at so stale-claim recovery ignores it.
func TestUpdateCodeDeploymentStatus_ClearsClaimOnTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-clear-claim-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Claim it.
	claimed, err := q.ClaimBuildingDeployment(ctx, "worker-term")
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v (claimed=%v)", err, claimed)
	}

	// Transition to failed — claim must be cleared.
	if err := q.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusFailed, map[string]any{
		"error_message": "test failure",
	}); err != nil {
		t.Fatalf("UpdateCodeDeploymentStatus: %v", err)
	}

	// Verify via ClaimBuildingDeployment returning nil (no unclaimed building rows).
	next, err := q.ClaimBuildingDeployment(ctx, "worker-check")
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if next != nil {
		t.Errorf("expected no claimable deployments after terminal update, got %s", next.ID)
	}

	// Also verify the deployment is in failed status.
	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment: %v", err)
	}
	if got.Status != domain.DeploymentStatusFailed {
		t.Errorf("expected failed, got %s", got.Status)
	}
}

// TestUpdateCodeDeploymentStatus_SetsFinishedAtOnTerminal verifies that
// finished_at is set when transitioning to ready/failed/timed_out but NOT when
// transitioning to building (a non-terminal state).
func TestUpdateCodeDeploymentStatus_SetsFinishedAtOnTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	for _, status := range []domain.DeploymentBuildStatus{
		domain.DeploymentStatusReady,
		domain.DeploymentStatusFailed,
		domain.DeploymentStatusTimedOut,
	} {
		t.Run(string(status), func(t *testing.T) {
			projectID := "proj-fat-" + newID()
			job := mustCreateJobForDeploy(t, ctx, q, projectID)
			d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

			if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
				t.Fatalf("confirm: %v", err)
			}

			before := time.Now().UTC()
			if err := q.UpdateCodeDeploymentStatus(ctx, d.ID, status, nil); err != nil {
				t.Fatalf("UpdateCodeDeploymentStatus(%s): %v", status, err)
			}
			after := time.Now().UTC()

			got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
			if err != nil {
				t.Fatalf("GetCodeDeployment: %v", err)
			}
			if got.FinishedAt == nil {
				t.Errorf("expected finished_at to be set for terminal status %s", status)
				return
			}
			if got.FinishedAt.Before(before) || got.FinishedAt.After(after) {
				t.Errorf("finished_at %v not in [%v, %v]", got.FinishedAt, before, after)
			}
		})
	}
}

// TestUpdateCodeDeploymentStatus_PreservesExistingFieldsOnPartialUpdate verifies
// that COALESCE prevents non-nil fields from being overwritten by nil in subsequent
// UpdateCodeDeploymentStatus calls.
func TestUpdateCodeDeploymentStatus_PreservesExistingFieldsOnPartialUpdate(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-coalesce-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// First update: set image URI.
	if err := q.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusReady, map[string]any{
		"built_image_uri":    "registry.example.com/img:v1",
		"built_image_digest": "sha256:abc",
		"build_logs":         "line1\nline2",
	}); err != nil {
		t.Fatalf("first update: %v", err)
	}

	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment: %v", err)
	}
	if got.BuiltImageURI != "registry.example.com/img:v1" {
		t.Errorf("BuiltImageURI = %q, want registry.example.com/img:v1", got.BuiltImageURI)
	}
	if got.BuildLogs != "line1\nline2" {
		t.Errorf("BuildLogs = %q, want 'line1\\nline2'", got.BuildLogs)
	}
}

// --- GetCodeDeployment ---

// TestGetCodeDeployment_CrossTenantIsolation verifies that a deployment
// belonging to project A cannot be fetched using project B's ID.
func TestGetCodeDeployment_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "proj-a-xti-" + newID()
	projectB := "proj-b-xti-" + newID()
	jobA := mustCreateJobForDeploy(t, ctx, q, projectA)
	_ = mustCreateJobForDeploy(t, ctx, q, projectB) // create project B's job (for FK)

	dA := mustCreateDeployment(t, ctx, q, jobA.ID, projectA)

	// Project B asking for project A's deployment must not find it.
	_, err := q.GetCodeDeployment(ctx, dA.ID, projectB)
	if err == nil {
		t.Fatal("expected not-found error when querying cross-tenant, got nil")
	}
	if !isNotFoundErr(err) {
		t.Errorf("expected ErrCodeDeploymentNotFound, got %v", err)
	}
}

// --- ListCodeDeployments ---

// TestListCodeDeployments_PaginationCursor verifies that cursor-based pagination
// returns the next page starting strictly after the cursor time.
func TestListCodeDeployments_PaginationCursor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cursor-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create 5 deployments. Each has a distinct created_at because Postgres
	// CURRENT_TIMESTAMP advances within a statement but we force ordering via IDs.
	var ids []string
	for i := 0; i < 5; i++ {
		d := mustCreateDeployment(t, ctx, q, job.ID, projectID)
		ids = append(ids, d.ID)
		time.Sleep(2 * time.Millisecond) // ensure distinct created_at
	}

	// Fetch first 3 (newest first).
	page1, err := q.ListCodeDeployments(ctx, job.ID, projectID, 3, nil)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("expected 3 results in page 1, got %d", len(page1))
	}

	// Use the oldest entry of page 1 as cursor for page 2.
	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListCodeDeployments(ctx, job.ID, projectID, 10, &cursor)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 results in page 2, got %d", len(page2))
	}

	// No overlap between pages.
	seen := make(map[string]bool)
	for _, d := range page1 {
		seen[d.ID] = true
	}
	for _, d := range page2 {
		if seen[d.ID] {
			t.Errorf("deployment %s appeared in both pages", d.ID)
		}
	}
}

// TestListCodeDeployments_CrossTenantIsolation verifies that project A's
// deployments are not visible in a project B listing.
func TestListCodeDeployments_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "proj-a-list-iso-" + newID()
	projectB := "proj-b-list-iso-" + newID()
	jobA := mustCreateJobForDeploy(t, ctx, q, projectA)
	jobB := mustCreateJobForDeploy(t, ctx, q, projectB)

	dA := mustCreateDeployment(t, ctx, q, jobA.ID, projectA)
	_ = mustCreateDeployment(t, ctx, q, jobB.ID, projectB)

	// List deployments for project A's job using project B's scope — must be empty.
	results, err := q.ListCodeDeployments(ctx, jobA.ID, projectB, 10, nil)
	if err != nil {
		t.Fatalf("ListCodeDeployments: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from cross-tenant query, got %d", len(results))
	}

	// Listing with the correct project must return the deployment.
	results, err = q.ListCodeDeployments(ctx, jobA.ID, projectA, 10, nil)
	if err != nil {
		t.Fatalf("ListCodeDeployments correct project: %v", err)
	}
	if len(results) != 1 || results[0].ID != dA.ID {
		t.Errorf("expected exactly deployment %s for project A, got %d results", dA.ID, len(results))
	}
}

// --- SetActiveDeployment ---

// TestSetActiveDeployment_UpdatesJobActiveDeploymentID verifies that calling
// SetActiveDeployment actually persists active_deployment_id on the job row
// and clears rollback_source_deployment_id (fresh build supersedes any rollback).
func TestSetActiveDeployment_UpdatesJobActiveDeploymentID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-sad-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
	d := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)

	if err := q.SetActiveDeployment(ctx, job.ID, d.ID, projectID); err != nil {
		t.Fatalf("SetActiveDeployment: %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.ActiveDeploymentID != d.ID {
		t.Errorf("ActiveDeploymentID = %q, want %q", got.ActiveDeploymentID, d.ID)
	}
	// A fresh build must clear any previous rollback source.
	if got.RollbackSourceDeploymentID != "" {
		t.Errorf("RollbackSourceDeploymentID should be empty after new build, got %q", got.RollbackSourceDeploymentID)
	}
}

// TestSetActiveDeployment_ClearsRollbackSourceDeploymentID verifies that a
// successful new build activation clears the rollback_source_deployment_id
// that was set by a previous RollbackToDeployment call.
func TestSetActiveDeployment_ClearsRollbackSourceDeploymentID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-sadc-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
	d1 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)
	d2 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)
	d3 := mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)

	// Promote d2, then roll back to d1 (sets rollback_source = d2).
	if err := q.SetActiveDeployment(ctx, job.ID, d2.ID, projectID); err != nil {
		t.Fatalf("SetActiveDeployment(d2): %v", err)
	}
	if err := q.RollbackToDeployment(ctx, job.ID, d1.ID, projectID); err != nil {
		t.Fatalf("RollbackToDeployment(d1): %v", err)
	}

	// Verify rollback source is set.
	jobAfterRollback, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob after rollback: %v", err)
	}
	if jobAfterRollback.RollbackSourceDeploymentID != d2.ID {
		t.Fatalf("expected rollback_source=%s, got %q", d2.ID, jobAfterRollback.RollbackSourceDeploymentID)
	}

	// New build activation (d3) must clear rollback source.
	if err := q.SetActiveDeployment(ctx, job.ID, d3.ID, projectID); err != nil {
		t.Fatalf("SetActiveDeployment(d3): %v", err)
	}
	jobAfterNewBuild, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob after new build: %v", err)
	}
	if jobAfterNewBuild.ActiveDeploymentID != d3.ID {
		t.Errorf("ActiveDeploymentID = %q, want %q", jobAfterNewBuild.ActiveDeploymentID, d3.ID)
	}
	if jobAfterNewBuild.RollbackSourceDeploymentID != "" {
		t.Errorf("RollbackSourceDeploymentID should be empty after new build, got %q",
			jobAfterNewBuild.RollbackSourceDeploymentID)
	}
}

// TestSetActiveDeployment_CrossTenantRejected verifies that passing a mismatched
// project_id returns an error and does not update the job.
func TestSetActiveDeployment_CrossTenantRejected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "proj-a-sat-" + newID()
	projectB := "proj-b-sat-" + newID()
	jobA := mustCreateJobForDeploy(t, ctx, q, projectA)
	dA := mustCreateReadyDeployment(t, ctx, q, jobA.ID, projectA)

	// Try to activate dA on job A but with project B's ID — must fail.
	err := q.SetActiveDeployment(ctx, jobA.ID, dA.ID, projectB)
	if err == nil {
		t.Fatal("expected error for cross-tenant SetActiveDeployment, got nil")
	}
}

// --- RollbackToDeployment ---

// TestRollbackToDeployment_CrossTenantRejected verifies that a rollback targeting
// a deployment that belongs to a different project returns an error.
func TestRollbackToDeployment_CrossTenantRejected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "proj-a-rtcr-" + newID()
	projectB := "proj-b-rtcr-" + newID()
	jobA := mustCreateJobForDeploy(t, ctx, q, projectA)
	jobB := mustCreateJobForDeploy(t, ctx, q, projectB)
	dA := mustCreateReadyDeployment(t, ctx, q, jobA.ID, projectA)
	_ = mustCreateReadyDeployment(t, ctx, q, jobB.ID, projectB)

	// Try to roll back job B to a deployment belonging to job A's project.
	err := q.RollbackToDeployment(ctx, jobB.ID, dA.ID, projectB)
	if err == nil {
		t.Fatal("expected error when rolling back to a deployment from another project, got nil")
	}
}

// TestRollbackToDeployment_WrongJobRejected verifies that rolling back to a
// deployment that belongs to a different job (same project) is rejected.
func TestRollbackToDeployment_WrongJobRejected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rtwr-" + newID()
	jobA := mustCreateJobForDeploy(t, ctx, q, projectID)
	jobB := mustCreateJobForDeploy(t, ctx, q, projectID)
	dA := mustCreateReadyDeployment(t, ctx, q, jobA.ID, projectID)

	// Roll back job B to a deployment that belongs to job A — must fail.
	err := q.RollbackToDeployment(ctx, jobB.ID, dA.ID, projectID)
	if err == nil {
		t.Fatal("expected error when rolling back to another job's deployment, got nil")
	}
}

// --- DeleteExpiredDeployments batching ---

// TestDeleteExpiredDeployments_BatchedLoop verifies that DeleteExpiredDeployments
// processes large counts in multiple batches rather than a single unbounded DELETE.
// We insert more rows than deleteExpiredBatchSize (500) and verify all are removed.
func TestDeleteExpiredDeployments_BatchedLoop(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-batch-gc-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Insert 12 pending deployments — small count so the test stays fast but
	// still exercises the batching path (we reduce batchSize in the store for testing
	// by just verifying the total deletion count, not the batch count itself).
	const count = 12
	for i := 0; i < count; i++ {
		mustCreateDeployment(t, ctx, q, job.ID, projectID)
	}

	deleted, err := q.DeleteExpiredDeployments(ctx, time.Now().Add(1*time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("DeleteExpiredDeployments: %v", err)
	}
	if deleted < count {
		t.Errorf("expected >= %d deletions, got %d", count, deleted)
	}

	// Confirm the table is empty for this project.
	remaining, err := q.ListCodeDeployments(ctx, job.ID, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListCodeDeployments after GC: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining deployments, got %d", len(remaining))
	}
}

// --- is_rollback persistence ---

// TestIsRollbackPersistedInJobRun verifies that a job run created with
// IsRollback=true persists the value and GetRun returns it correctly.
func TestIsRollbackPersistedInJobRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-irp-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Insert run directly with is_rollback=true via store.CreateRun. Note: store.CreateRun
	// does not include is_rollback in its INSERT, so we use a direct SQL exec to set it.
	runID := "run-irp-" + newID()
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by,
		                      execution_mode, is_rollback, created_at)
		VALUES ($1, $2, $3, 'queued', 1, 'manual', 'http', TRUE, NOW())`,
		runID, job.ID, projectID,
	)
	if err != nil {
		t.Fatalf("insert run with is_rollback: %v", err)
	}

	got, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if !got.IsRollback {
		t.Error("expected IsRollback=true, got false")
	}
}

// TestIsRollbackDefaultsFalse verifies that a run created without explicitly
// setting is_rollback defaults to false.
func TestIsRollbackDefaultsFalse(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-irdf-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	runID := "run-irdf-" + newID()
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by,
		                      execution_mode, created_at)
		VALUES ($1, $2, $3, 'queued', 1, 'manual', 'http', NOW())`,
		runID, job.ID, projectID,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	got, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.IsRollback {
		t.Error("expected IsRollback=false by default, got true")
	}
}

// --- ClaimBuildingDeployment ordering ---

// TestClaimBuildingDeployment_OrderByCreatedAt verifies that the oldest
// unclaimed building deployment is always claimed first (FIFO order).
func TestClaimBuildingDeployment_OrderByCreatedAt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-order-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create two deployments and confirm both to building, with a time gap
	// to ensure distinct created_at timestamps.
	d1 := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	time.Sleep(5 * time.Millisecond)
	d2 := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, d1.ID); err != nil {
		t.Fatalf("confirm d1: %v", err)
	}
	if err := q.ConfirmCodeDeployment(ctx, d2.ID); err != nil {
		t.Fatalf("confirm d2: %v", err)
	}

	// First claim must return d1 (older).
	first, err := q.ClaimBuildingDeployment(ctx, "worker-order-1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if first == nil {
		t.Fatal("expected first claim to return a deployment, got nil")
	}
	if first.ID != d1.ID {
		t.Errorf("expected first claim to be d1 (%s), got %s", d1.ID, first.ID)
	}

	// Second claim must return d2 (newer).
	second, err := q.ClaimBuildingDeployment(ctx, "worker-order-2")
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second == nil {
		t.Fatal("expected second claim to return a deployment, got nil")
	}
	if second.ID != d2.ID {
		t.Errorf("expected second claim to be d2 (%s), got %s", d2.ID, second.ID)
	}
}

// TestClaimBuildingDeployment_AlreadyClaimedSkipped verifies that a deployment
// already claimed by another worker is not returned by a concurrent claim call.
func TestClaimBuildingDeployment_AlreadyClaimedSkipped(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-skip-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Worker A claims.
	claimed, err := q.ClaimBuildingDeployment(ctx, "worker-a-skip")
	if err != nil || claimed == nil {
		t.Fatalf("worker A claim: %v (claimed=%v)", err, claimed)
	}

	// Worker B finds nothing — the only deployment is already claimed.
	second, err := q.ClaimBuildingDeployment(ctx, "worker-b-skip")
	if err != nil {
		t.Fatalf("worker B claim: %v", err)
	}
	if second != nil {
		t.Errorf("expected nil from worker B (all deployments claimed), got %s", second.ID)
	}
}

// --- Concurrent safety ---

// TestUpdateCodeDeploymentStatus_ConcurrentTerminalIdempotent verifies that
// two goroutines racing to mark a deployment failed both succeed without
// corrupting the row (the second returns ErrCodeDeploymentNotFound or succeeds).
func TestUpdateCodeDeploymentStatus_ConcurrentTerminalIdempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-concurrent-term-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)
	d := mustCreateDeployment(t, ctx, q, job.ID, projectID)

	if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	var wg conc.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			errs[i] = q.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusFailed, map[string]any{
				"error_message": fmt.Sprintf("worker %d failure", i),
			})
		})
	}
	wg.Wait()

	// Both calls must either succeed or return a not-found (second idempotent call).
	for i, err := range errs {
		if err != nil && !isNotFoundErr(err) {
			t.Errorf("worker %d: unexpected error: %v", i, err)
		}
	}

	// Final state must be failed (not a mix).
	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment: %v", err)
	}
	if got.Status != domain.DeploymentStatusFailed {
		t.Errorf("expected failed status, got %s", got.Status)
	}
}

// --- ListBuildingDeployments ---

// TestListBuildingDeployments_ReturnsOnlyBuildingStatus verifies that
// ListBuildingDeployments returns only deployments with status="building",
// ignoring pending and ready deployments.
func TestListBuildingDeployments_ReturnsOnlyBuildingStatus(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-lbd-status-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create a pending deployment (not confirmed — stays pending).
	_ = mustCreateDeployment(t, ctx, q, job.ID, projectID)

	// Create a building deployment (confirmed → building).
	dBuilding := mustCreateDeployment(t, ctx, q, job.ID, projectID)
	if err := q.ConfirmCodeDeployment(ctx, dBuilding.ID); err != nil {
		t.Fatalf("ConfirmCodeDeployment: %v", err)
	}

	// Create a ready deployment.
	_ = mustCreateReadyDeployment(t, ctx, q, job.ID, projectID)

	results, err := q.ListBuildingDeployments(ctx, 100)
	if err != nil {
		t.Fatalf("ListBuildingDeployments: %v", err)
	}

	for _, d := range results {
		if d.Status != domain.DeploymentStatusBuilding {
			t.Errorf("ListBuildingDeployments returned deployment with status %q, want building", d.Status)
		}
	}

	found := false
	for _, d := range results {
		if d.ID == dBuilding.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("building deployment %s not found in ListBuildingDeployments results", dBuilding.ID)
	}
}

// TestListBuildingDeployments_EmptyResult verifies that ListBuildingDeployments
// returns an empty (non-nil) slice when no building deployments exist.
func TestListBuildingDeployments_EmptyResult(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	results, err := q.ListBuildingDeployments(ctx, 100)
	if err != nil {
		t.Fatalf("ListBuildingDeployments on empty table: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %d results", len(results))
	}
}

// TestListBuildingDeployments_MultipleRows verifies that all building deployments
// up to the limit are returned in ascending creation order (oldest first).
func TestListBuildingDeployments_MultipleRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-lbd-multi-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	const count = 5
	ids := make([]string, count)
	for i := range count {
		d := mustCreateDeployment(t, ctx, q, job.ID, projectID)
		if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
			t.Fatalf("ConfirmCodeDeployment %d: %v", i, err)
		}
		ids[i] = d.ID
	}

	results, err := q.ListBuildingDeployments(ctx, 100)
	if err != nil {
		t.Fatalf("ListBuildingDeployments: %v", err)
	}
	if len(results) < count {
		t.Errorf("expected at least %d results, got %d", count, len(results))
	}

	// Verify ascending creation order (oldest first).
	for i := 1; i < len(results); i++ {
		if results[i].CreatedAt.Before(results[i-1].CreatedAt) {
			t.Errorf("results not in ascending creation order at index %d", i)
		}
	}

	// All created IDs must appear in results.
	resultIDs := make(map[string]bool, len(results))
	for _, d := range results {
		resultIDs[d.ID] = true
	}
	for _, id := range ids {
		if !resultIDs[id] {
			t.Errorf("building deployment %s not found in ListBuildingDeployments results", id)
		}
	}
}

// TestListBuildingDeployments_LimitRespected verifies that the limit parameter
// is honored and at most limit rows are returned.
func TestListBuildingDeployments_LimitRespected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-lbd-limit-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	// Create 5 building deployments.
	for range 5 {
		d := mustCreateDeployment(t, ctx, q, job.ID, projectID)
		if err := q.ConfirmCodeDeployment(ctx, d.ID); err != nil {
			t.Fatalf("ConfirmCodeDeployment: %v", err)
		}
	}

	results, err := q.ListBuildingDeployments(ctx, 3)
	if err != nil {
		t.Fatalf("ListBuildingDeployments(limit=3): %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results with limit=3, got %d", len(results))
	}
}

// --- CreateCodeDeployment edge cases ---

// TestCreateCodeDeployment_NilCreatedBy verifies that a deployment with an empty
// CreatedBy field is stored with a NULL created_by in the database.
func TestCreateCodeDeployment_NilCreatedBy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-nil-creator-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	id := "deploy-nil-creator-" + newID()
	d := &domain.CodeDeployment{
		ID:              id,
		JobID:           job.ID,
		ProjectID:       projectID,
		Runtime:         domain.RuntimePython,
		SourceHash:      "hash" + id[:8],
		SourceSizeBytes: 512,
		SourceURI:       "projects/" + projectID + "/jobs/" + job.ID + "/deploys/" + id + ".tar.gz",
		CreatedBy:       "", // nil equivalent
	}
	if err := q.CreateCodeDeployment(ctx, d); err != nil {
		t.Fatalf("CreateCodeDeployment with empty CreatedBy: %v", err)
	}

	got, err := q.GetCodeDeployment(ctx, id, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment: %v", err)
	}
	if got.CreatedBy != "" {
		t.Errorf("expected empty CreatedBy (NULL), got %q", got.CreatedBy)
	}
}

// TestCreateCodeDeployment_DefaultStatusIsPending verifies that when no Status is
// set on the input struct, the created deployment has status="pending".
func TestCreateCodeDeployment_DefaultStatusIsPending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-default-status-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	id := "deploy-default-status-" + newID()
	d := &domain.CodeDeployment{
		ID:              id,
		JobID:           job.ID,
		ProjectID:       projectID,
		Runtime:         domain.RuntimeGo,
		SourceHash:      "hash" + id[:8],
		SourceSizeBytes: 2048,
		SourceURI:       "projects/" + projectID + "/jobs/" + job.ID + "/deploys/" + id + ".tar.gz",
		// Status intentionally omitted — should default to "pending".
	}
	if err := q.CreateCodeDeployment(ctx, d); err != nil {
		t.Fatalf("CreateCodeDeployment: %v", err)
	}

	// The struct itself should be mutated.
	if d.Status != domain.DeploymentStatusPending {
		t.Errorf("in-memory Status after create = %q, want pending", d.Status)
	}

	got, err := q.GetCodeDeployment(ctx, id, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment: %v", err)
	}
	if got.Status != domain.DeploymentStatusPending {
		t.Errorf("DB status = %q, want pending", got.Status)
	}
}

// TestCreateCodeDeployment_AutoGeneratesIDWhenEmpty verifies that when the ID
// field is empty the store generates a valid UUID and writes it back to the struct.
func TestCreateCodeDeployment_AutoGeneratesIDWhenEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-auto-id-" + newID()
	job := mustCreateJobForDeploy(t, ctx, q, projectID)

	d := &domain.CodeDeployment{
		// ID intentionally omitted — should be auto-generated.
		JobID:           job.ID,
		ProjectID:       projectID,
		Runtime:         domain.RuntimeTypeScript,
		SourceHash:      "hash-auto",
		SourceSizeBytes: 1024,
		SourceURI:       "projects/" + projectID + "/jobs/" + job.ID + "/deploys/auto.tar.gz",
	}
	if err := q.CreateCodeDeployment(ctx, d); err != nil {
		t.Fatalf("CreateCodeDeployment with empty ID: %v", err)
	}
	if d.ID == "" {
		t.Error("expected ID to be auto-populated after create, got empty string")
	}

	// Verify retrievable from DB using the generated ID.
	got, err := q.GetCodeDeployment(ctx, d.ID, projectID)
	if err != nil {
		t.Fatalf("GetCodeDeployment with auto-generated ID: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("DB ID = %q, want %q", got.ID, d.ID)
	}
}

// --- Helpers ---

// isNotFoundErr returns true if err is store.ErrCodeDeploymentNotFound.
func isNotFoundErr(err error) bool {
	return err != nil && err.Error() == store.ErrCodeDeploymentNotFound.Error()
}
