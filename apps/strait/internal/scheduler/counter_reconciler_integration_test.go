//go:build integration

package scheduler_test

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

func setupReconciler(t *testing.T) (*testutil.TestDB, *store.Queries, *queue.PostgresQueue, *domain.Job) {
	t.Helper()
	ctx := context.Background()
	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup db: %v", err)
	}
	t.Cleanup(func() { tdb.Cleanup(ctx) })
	st := store.New(tdb.Pool)
	q := queue.NewPostgresQueue(tdb.Pool)

	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   "recon-" + uuid.Must(uuid.NewV7()).String(),
		Name:        "recon-job",
		Slug:        "recon-" + uuid.Must(uuid.NewV7()).String()[:8],
		EndpointURL: "https://example.com/x",
		MaxAttempts: 3,
		TimeoutSecs: 60,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return tdb, st, q, job
}

func TestCounterReconciler_HappyPath_ZeroDrift(t *testing.T) {
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	for range 5 {
		r := &domain.JobRun{
			ID:        uuid.Must(uuid.NewV7()).String(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  1,
		}
		if err := q.Enqueue(ctx, r); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	if _, err := q.DequeueN(ctx, 3); err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		r.Run(runCtx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if r.TotalDrift() != 0 {
		t.Errorf("drift on clean DB = %d, want 0", r.TotalDrift())
	}
}

func TestCounterReconciler_InducedDrift_ActiveCounts(t *testing.T) {
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	for range 5 {
		r := &domain.JobRun{
			ID:        uuid.Must(uuid.NewV7()).String(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
		}
		if err := q.Enqueue(ctx, r); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	if _, err := q.DequeueN(ctx, 5); err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	// Manually corrupt the counter to simulate drift.
	_, err := tdb.Pool.Exec(ctx,
		`UPDATE job_active_counts SET count = count + 10 WHERE job_id = $1`,
		job.ID,
	)
	if err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	if err := r.RunOnceForTest(ctx); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	// Verify the counter is now correct.
	var count int
	err = tdb.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,
		job.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 5 {
		t.Errorf("counter after reconcile = %d, want 5", count)
	}
	if r.TotalDrift() < 10 {
		t.Errorf("drift = %d, want >= 10", r.TotalDrift())
	}
}

func TestCounterReconciler_InducedDrift_DLQCounts(t *testing.T) {
	tdb, _, _, job := setupReconciler(t)
	ctx := context.Background()

	// Directly insert dead_letter rows.
	for range 3 {
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW())
		`, uuid.Must(uuid.NewV7()).String(), job.ID, job.ProjectID)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Corrupt the dlq counter.
	_, err := tdb.Pool.Exec(ctx,
		`UPDATE dlq_counts SET count = 100 WHERE job_id = $1`,
		job.ID,
	)
	if err != nil {
		t.Fatalf("corrupt dlq: %v", err)
	}

	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	if err := r.RunOnceForTest(ctx); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	var count int
	err = tdb.Pool.QueryRow(ctx, `SELECT count FROM dlq_counts WHERE job_id = $1`, job.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 3 {
		t.Errorf("dlq count after reconcile = %d, want 3", count)
	}
}

func TestCounterReconciler_BypassTriggerRepaired(t *testing.T) {
	tdb, _, _, job := setupReconciler(t)
	ctx := context.Background()

	// Disable the trigger, insert rows (counter unchanged), re-enable.
	_, err := tdb.Pool.Exec(ctx, `ALTER TABLE job_runs DISABLE TRIGGER job_runs_active_counts_trg`)
	if err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	for range 4 {
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, started_at)
			VALUES ($1, $2, $3, 'executing', 1, 'manual', NOW(), NOW())
		`, uuid.Must(uuid.NewV7()).String(), job.ID, job.ProjectID)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	_, err = tdb.Pool.Exec(ctx, `ALTER TABLE job_runs ENABLE TRIGGER job_runs_active_counts_trg`)
	if err != nil {
		t.Fatalf("enable trigger: %v", err)
	}

	// Counter should be 0 (trigger was off during inserts).
	var before int
	_ = tdb.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&before)
	if before != 0 {
		t.Fatalf("counter before reconcile = %d, want 0", before)
	}

	// Reconcile.
	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	if err := r.RunOnceForTest(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var after int
	_ = tdb.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM job_active_counts WHERE job_id=$1`, job.ID).Scan(&after)
	if after != 4 {
		t.Errorf("counter after reconcile = %d, want 4", after)
	}
}

// TestCounterReconciler_PropertyRandomOps runs a random sequence of queue
// operations and asserts that after reconcile the counters always equal
// ground truth.
func TestCounterReconciler_PropertyRandomOps(t *testing.T) {
	tdb, _, q, job := setupReconciler(t)
	ctx := context.Background()

	rng := rand.New(rand.NewPCG(42, 42))
	var runIDs []string
	const ops = 200

	for range ops {
		switch rng.IntN(5) {
		case 0: // enqueue
			r := &domain.JobRun{
				ID:        uuid.Must(uuid.NewV7()).String(),
				JobID:     job.ID,
				ProjectID: job.ProjectID,
			}
			if err := q.Enqueue(ctx, r); err == nil {
				runIDs = append(runIDs, r.ID)
			}
		case 1: // dequeue
			_, _ = q.DequeueN(ctx, 1+rng.IntN(3))
		case 2: // complete a random dequeued run
			if len(runIDs) > 0 {
				id := runIDs[rng.IntN(len(runIDs))]
				_, _ = tdb.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1 AND status IN ('dequeued','executing')`, id)
			}
		case 3: // fail to dead_letter
			if len(runIDs) > 0 {
				id := runIDs[rng.IntN(len(runIDs))]
				_, _ = tdb.Pool.Exec(ctx, `UPDATE job_runs SET status='dead_letter', finished_at=NOW() WHERE id=$1 AND status='queued'`, id)
			}
		case 4: // mask a dlq row
			_, _ = tdb.Pool.Exec(ctx, `UPDATE job_runs SET visible_until=NOW() WHERE status='dead_letter' AND visible_until IS NULL AND job_id=$1`, job.ID)
		}
	}

	// Reconcile and assert zero drift (meaning the trigger stayed correct).
	r := scheduler.NewCounterReconciler(tdb.Pool, scheduler.CounterReconcilerConfig{})
	if err := r.RunOnceForTest(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// Since triggers maintain the counters, reconciler should report no drift.
	// If the triggers had a bug we'd see drift > 0.
	if r.TotalDrift() != 0 {
		t.Errorf("drift after random ops = %d, want 0 (trigger should maintain invariant)", r.TotalDrift())
	}
}
