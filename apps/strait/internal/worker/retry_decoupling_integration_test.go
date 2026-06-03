//go:build integration

package worker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestRetry_WritesSideTableNotJobRuns verifies that after a transient failure
// causes a retry, the retry timestamp lives in job_retries and
// job_runs.next_retry_at is left untouched.
func TestRetry_WritesSideTableNotJobRuns(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := newWorkerQueue(t, env)
	job := mustCreateJob(t, ctx, st, "project-retry-side-table", srv.URL)
	job.MaxAttempts = 3
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	exec, _ := newExecutor(t, env, srv.URL, 2, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for retry to be scheduled")
		default:
		}

		var hasSideTableRow bool
		if err := env.DB.Pool.QueryRow(ctx,
			`SELECT strait_run_retry_blocked($1)`,
			run.ID,
		).Scan(&hasSideTableRow); err != nil {
			t.Fatalf("query job_retries: %v", err)
		}
		if !hasSideTableRow {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var jobRunsRetry *time.Time
		if err := env.DB.Pool.QueryRow(ctx,
			`SELECT next_retry_at FROM job_runs WHERE id = $1`, run.ID,
		).Scan(&jobRunsRetry); err != nil {
			t.Fatalf("query job_runs.next_retry_at: %v", err)
		}
		if jobRunsRetry != nil {
			t.Fatalf("job_runs.next_retry_at must be NULL on retry path; got %v", *jobRunsRetry)
		}
		return
	}
}

// TestRetry_DequeueRespectsSideTableSchedule verifies the dequeue predicate
// honors a future retry stored in job_retries: the run is not claimable
// before the timestamp passes, and becomes claimable after.
func TestRetry_DequeueRespectsSideTableSchedule(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := newWorkerQueue(t, env)
	job := mustCreateJob(t, ctx, st, "project-retry-dequeue-gate", srv.URL)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
		VALUES ($1, NOW() + INTERVAL '2 seconds', 1, NOW())`, run.ID); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	batch, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(batch) != 0 {
		t.Fatalf("run should not be claimable before retry fires; got %d", len(batch))
	}

	time.Sleep(3 * time.Second)

	promoted, err := q.ActivateDueRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ActivateDueRuns after wait: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("ActivateDueRuns promoted = %d, want 1", promoted)
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick after retry promotion: %v", err)
	}

	var batch2 []domain.JobRun
	deadline := time.Now().Add(5 * time.Second)
	for len(batch2) == 0 && time.Now().Before(deadline) {
		if err := q.ForceTick(ctx, "http"); err != nil {
			t.Fatalf("ForceTick while waiting for promoted retry: %v", err)
		}
		var dequeueErr error
		batch2, dequeueErr = q.DequeueN(ctx, 1)
		if dequeueErr != nil {
			t.Fatalf("DequeueN after wait: %v", dequeueErr)
		}
		if len(batch2) == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if len(batch2) != 1 || batch2[0].ID != run.ID {
		var blocked bool
		var readyEvents, readyEmits, activeClaims int
		var latestRetryCleared bool
		if stateErr := env.DB.Pool.QueryRow(ctx, `
			SELECT
				strait_run_retry_blocked($1),
				(SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = $1 AND reason = 'retry_ready'),
				(SELECT COUNT(*) FROM strait_pgque_ready_events WHERE run_id = $1),
				(SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1),
				COALESCE((SELECT cleared FROM job_retries WHERE run_id = $1 ORDER BY id DESC LIMIT 1), FALSE)`,
			run.ID,
		).Scan(&blocked, &readyEvents, &readyEmits, &activeClaims, &latestRetryCleared); stateErr != nil {
			t.Fatalf("query retry claim state: %v", stateErr)
		}
		t.Fatalf(
			"expected to claim %s after retry fires, got %v; blocked=%v ready_events=%d ready_emits=%d active_claims=%d latest_retry_cleared=%v",
			run.ID,
			batch2,
			blocked,
			readyEvents,
			readyEmits,
			activeClaims,
			latestRetryCleared,
		)
	}
}

// TestRetry_ClearOnTerminal verifies that when a run reaches a terminal
// state via the executor, its job_retries row (if any) is no longer a
// dequeue gate. We assert dequeue claims behave correctly even with a
// stale retry row present.
func TestRetry_ClearOnTerminal(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := newWorkerQueue(t, env)
	job := mustCreateJob(t, ctx, st, "project-retry-clear-terminal", srv.URL)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	exec, _ := newExecutor(t, env, srv.URL, 2, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for terminal completion")
		default:
		}
		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if got.Status == domain.StatusCompleted {
			return
		}
		if got.Status.IsTerminal() {
			t.Fatalf("unexpected terminal status %q", got.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestRetry_IndexDropped verifies migration 000254 actually dropped the
// idx_job_runs_retry / idx_runs_retry partial indexes that previously
// gated the dequeue path on job_runs.next_retry_at. With the side table
// authoritative, those indexes were write amplification.
func TestRetry_IndexDropped(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	for _, idx := range []string{"idx_job_runs_retry", "idx_runs_retry"} {
		var exists bool
		if err := env.DB.Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx,
		).Scan(&exists); err != nil {
			t.Fatalf("query pg_indexes for %s: %v", idx, err)
		}
		if exists {
			t.Errorf("index %s must be dropped by migration 000254; still present", idx)
		}
	}
}
