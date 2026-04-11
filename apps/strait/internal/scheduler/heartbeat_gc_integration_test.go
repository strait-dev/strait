//go:build integration

package scheduler_test

import (
	"context"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

func setupHeartbeatGC(t *testing.T) (*testutil.TestDB, *store.Queries) {
	t.Helper()
	ctx := context.Background()
	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { tdb.Cleanup(ctx) })
	return tdb, store.New(tdb.Pool)
}

func hbGCMakeJob(t *testing.T, st *store.Queries, ctx context.Context, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   projectID,
		Name:        "gc-job",
		Slug:        "gc-" + uuid.Must(uuid.NewV7()).String()[:8],
		EndpointURL: "https://example.com/x",
		MaxAttempts: 3,
		TimeoutSecs: 60,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func TestHeartbeatGC_DeletesOrphansPreservesLive(t *testing.T) {
	tdb, st := setupHeartbeatGC(t)
	ctx := context.Background()

	// Create a job and two runs.
	projectID := "gc-" + uuid.Must(uuid.NewV7()).String()
	_ = tdb.Pool.QueryRow(ctx, `SELECT 1`)
	job := hbGCMakeJob(t, st, ctx, projectID)

	liveID := uuid.Must(uuid.NewV7()).String()
	orphanID := uuid.Must(uuid.NewV7()).String()

	// Insert two runs, one executing (live heartbeat) and one completed (orphan).
	for _, id := range []string{liveID, orphanID} {
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, started_at)
			VALUES ($1, $2, $3, 'executing', 1, 'manual', NOW(), NOW())
		`, id, job.ID, job.ProjectID)
		if err != nil {
			t.Fatalf("insert run %s: %v", id, err)
		}
	}
	// Register both heartbeats.
	if err := st.BatchUpsertHeartbeatSideTable(ctx, []string{liveID, orphanID}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Transition the orphan to completed.
	_, err := tdb.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, orphanID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Run GC.
	gc := scheduler.NewHeartbeatGC(st, scheduler.HeartbeatGCConfig{})
	if err := gc.RunOnceForTest(ctx); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if gc.TotalDeleted() != 1 {
		t.Errorf("deleted = %d, want 1", gc.TotalDeleted())
	}

	// live row still present.
	var count int
	_ = tdb.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`, liveID).Scan(&count)
	if count != 1 {
		t.Errorf("live heartbeat count = %d, want 1", count)
	}
	// orphan gone.
	_ = tdb.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`, orphanID).Scan(&count)
	if count != 0 {
		t.Errorf("orphan heartbeat count = %d, want 0", count)
	}
}

func TestHeartbeatGC_BatchLimitRespected(t *testing.T) {
	tdb, st := setupHeartbeatGC(t)
	ctx := context.Background()

	projectID := "gc-batch-" + uuid.Must(uuid.NewV7()).String()
	job := hbGCMakeJob(t, st, ctx, projectID)
	// Create 10 orphan heartbeats.
	var ids []string
	for i := 0; i < 10; i++ {
		id := uuid.Must(uuid.NewV7()).String()
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'completed', 1, 'manual', NOW(), NOW())
		`, id, job.ID, job.ProjectID)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		ids = append(ids, id)
	}
	if err := st.BatchUpsertHeartbeatSideTable(ctx, ids); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	gc := scheduler.NewHeartbeatGC(st, scheduler.HeartbeatGCConfig{BatchLimit: 5})
	_ = gc.RunOnceForTest(ctx)
	if gc.TotalDeleted() != 5 {
		t.Errorf("first tick deleted = %d, want 5", gc.TotalDeleted())
	}
	_ = gc.RunOnceForTest(ctx)
	if gc.TotalDeleted() != 10 {
		t.Errorf("second tick total = %d, want 10", gc.TotalDeleted())
	}
}

func TestEnsureQueueTriggersPresent_Happy(t *testing.T) {
	tdb, _ := setupHeartbeatGC(t)
	if err := scheduler.EnsureQueueTriggersPresent(context.Background(), tdb.Pool); err != nil {
		t.Errorf("expected triggers present, got %v", err)
	}
}

func TestEnsureQueueTriggersPresent_MissingFailsLoud(t *testing.T) {
	tdb, _ := setupHeartbeatGC(t)
	ctx := context.Background()
	// Drop one trigger and assert the check fails.
	_, err := tdb.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS job_runs_notify_queue_wake ON job_runs`)
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	err = scheduler.EnsureQueueTriggersPresent(ctx, tdb.Pool)
	if err == nil || !strings.Contains(err.Error(), "job_runs_notify_queue_wake") {
		t.Errorf("expected missing-trigger error, got %v", err)
	}
}
