//go:build integration

package scheduler_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

type promoterEnqueuer interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

func setupForPromoter(t *testing.T) (*testutil.TestDB, *store.Queries, promoterEnqueuer) {
	t.Helper()
	ctx := context.Background()
	tdb := getTestDB(t)
	intTestClean(t, ctx)
	st := store.New(tdb.Pool)
	q := intTestQueue(t)
	return tdb, st, q
}

func createJobAndQueuedRuns(t *testing.T, st *store.Queries, q promoterEnqueuer, n int, priority int, ageBackdate time.Duration) []*domain.JobRun {
	t.Helper()
	ctx := context.Background()
	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   "promoter-test-" + uuid.Must(uuid.NewV7()).String(),
		Name:        "promoter-job",
		Slug:        "promoter-job-" + uuid.Must(uuid.NewV7()).String()[:8],
		EndpointURL: "https://example.com/x",
		MaxAttempts: 3,
		TimeoutSecs: 60,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	runs := make([]*domain.JobRun, 0, n)
	for range n {
		r := &domain.JobRun{
			ID:        uuid.Must(uuid.NewV7()).String(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  priority,
		}
		if err := q.Enqueue(ctx, r); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		runs = append(runs, r)
	}
	return runs
}

func TestPriorityPromoter_PromotesAgedRuns(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	tdb, st, q := setupForPromoter(t)
	ctx := context.Background()

	runs := createJobAndQueuedRuns(t, st, q, 5, 10, 0)

	// Backdate the rows so they are older than the threshold.
	for _, r := range runs {
		_, err := tdb.Pool.Exec(ctx, `
			UPDATE job_runs SET created_at = NOW() - INTERVAL '10 minutes' WHERE id = $1
		`, r.ID)
		if err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	p := scheduler.NewPriorityPromoter(tdb.Pool, scheduler.PriorityPromoterConfig{
		AgeThreshold: 5 * time.Minute,
		MaxPriority:  100,
		BatchLimit:   1000,
	})
	// Use the unexported runOnce via a helper: run the full Run for a tick
	// then cancel.
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(runCtx)
		close(done)
	})
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	var maxStatePriority int
	if err := tdb.Pool.QueryRow(ctx, `SELECT MAX(priority) FROM job_run_state WHERE job_id = $1`, runs[0].JobID).Scan(&maxStatePriority); err != nil {
		t.Fatalf("query state priority: %v", err)
	}
	if maxStatePriority != 10 {
		t.Errorf("max state priority after promotion = %d, want unchanged 10", maxStatePriority)
	}

	var maxReadPriority int
	if err := tdb.Pool.QueryRow(ctx, `
		SELECT MAX(priority)
		FROM job_run_read_state
		WHERE job_id = $1`,
		runs[0].JobID,
	).Scan(&maxReadPriority); err != nil {
		t.Fatalf("query read priority: %v", err)
	}
	if maxReadPriority != 11 {
		t.Errorf("max read priority after promotion = %d, want 11", maxReadPriority)
	}

	var maxLedgerPriority int
	if err := tdb.Pool.QueryRow(ctx, `SELECT MAX(priority) FROM job_runs WHERE job_id = $1`, runs[0].JobID).Scan(&maxLedgerPriority); err != nil {
		t.Fatalf("query ledger priority: %v", err)
	}
	if maxLedgerPriority != 10 {
		t.Errorf("max ledger priority after promotion = %d, want immutable ledger value 10", maxLedgerPriority)
	}

	var priorityEvents int
	if err := tdb.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_priority_events e
		JOIN job_runs jr ON jr.id = e.run_id
		WHERE jr.job_id = $1`,
		runs[0].JobID,
	).Scan(&priorityEvents); err != nil {
		t.Fatalf("query priority events: %v", err)
	}
	if priorityEvents != len(runs) {
		t.Errorf("priority events = %d, want %d", priorityEvents, len(runs))
	}
	var maxClaimPri int
	if err := tdb.Pool.QueryRow(ctx, `SELECT MAX(priority) FROM job_run_queue WHERE job_id = $1`, runs[0].JobID).Scan(&maxClaimPri); err != nil {
		t.Fatalf("query claim priority: %v", err)
	}
	if maxClaimPri != 10 {
		t.Errorf("max queue priority after promotion = %d, want immutable base value 10", maxClaimPri)
	}
}

func TestPriorityPromoter_RespectsCeiling(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	tdb, st, q := setupForPromoter(t)
	ctx := context.Background()

	runs := createJobAndQueuedRuns(t, st, q, 3, 1000, 0)
	// Backdate them.
	for _, r := range runs {
		_, err := tdb.Pool.Exec(ctx, `UPDATE job_runs SET created_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, r.ID)
		if err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	p := scheduler.NewPriorityPromoter(tdb.Pool, scheduler.PriorityPromoterConfig{
		AgeThreshold: time.Second,
		MaxPriority:  1000,
		BatchLimit:   100,
	})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(runCtx)
		close(done)
	})
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	var maxPri int
	if err := tdb.Pool.QueryRow(ctx, `SELECT MAX(priority) FROM job_run_read_state WHERE job_id = $1`, runs[0].JobID).Scan(&maxPri); err != nil {
		t.Fatalf("query priority: %v", err)
	}
	if maxPri != 1000 {
		t.Errorf("max priority after promotion = %d, want 1000 (ceiling)", maxPri)
	}
}

func TestPriorityPromoter_DoesNotTouchFresh(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	tdb, st, q := setupForPromoter(t)
	ctx := context.Background()

	runs := createJobAndQueuedRuns(t, st, q, 5, 10, 0)

	p := scheduler.NewPriorityPromoter(tdb.Pool, scheduler.PriorityPromoterConfig{
		AgeThreshold: 1 * time.Hour, // nothing is this old
		MaxPriority:  1000,
		BatchLimit:   100,
	})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(runCtx)
		close(done)
	})
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	var maxPri int
	if err := tdb.Pool.QueryRow(ctx, `SELECT MAX(priority) FROM job_run_read_state WHERE job_id = $1`, runs[0].JobID).Scan(&maxPri); err != nil {
		t.Fatalf("query: %v", err)
	}
	if maxPri != 10 {
		t.Errorf("priority changed unexpectedly: %d", maxPri)
	}
}

func TestPriorityPromoter_StarvedLowPriorityEventuallyDequeued(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	tdb, st, q := setupForPromoter(t)
	ctx := context.Background()

	// Create 5 high-priority fresh runs and 1 low-priority old run.
	highRuns := createJobAndQueuedRuns(t, st, q, 5, 100, 0)
	_ = highRuns
	// Use the same job for the old run so dequeue order is deterministic.
	// Rather than reusing, just create another job to keep isolation clean.
	oldRuns := createJobAndQueuedRuns(t, st, q, 1, 1, 0)
	// Backdate the low-priority one.
	_, err := tdb.Pool.Exec(ctx, `UPDATE job_runs SET created_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, oldRuns[0].ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Run the promoter enough times to bump the low-priority run above the
	// high-priority ones. With BatchLimit=1000 and MaxPriority=200 we need
	// at most 100 ticks.
	p := scheduler.NewPriorityPromoter(tdb.Pool, scheduler.PriorityPromoterConfig{
		AgeThreshold: 5 * time.Minute,
		MaxPriority:  200,
		BatchLimit:   1000,
		Interval:     5 * time.Millisecond,
	})
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(runCtx)
		close(done)
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var pri int
		if err := tdb.Pool.QueryRow(ctx, `SELECT priority FROM job_run_read_state WHERE run_id = $1`, oldRuns[0].ID).Scan(&pri); err != nil {
			t.Fatalf("query: %v", err)
		}
		if pri > 100 {
			cancel()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("starved run did not get promoted above high-priority runs in time")
}
