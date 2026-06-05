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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, st.UpdateJob(ctx,
		job))

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))

	exec, _ := newExecutor(t, env, srv.URL, 2, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for retry to be scheduled")
		default:
		}

		var hasSideTableRow bool
		require.NoError(t, env.DB.
			Pool.QueryRow(ctx,
			`SELECT strait_run_retry_blocked($1)`,

			run.ID).Scan(&hasSideTableRow))

		if !hasSideTableRow {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var jobRunsRetry *time.Time
		require.NoError(t, env.DB.
			Pool.QueryRow(ctx,
			`SELECT next_retry_at FROM job_runs WHERE id = $1`,

			run.ID).
			Scan(&jobRunsRetry))
		require.Nil(t, jobRunsRetry)

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
	require.NoError(t, q.Enqueue(ctx,
		run))

	if _, err := env.DB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
		VALUES ($1, NOW() + INTERVAL '2 seconds', 1, NOW())`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"schedule retry: %v", err)
	}

	batch, err := q.DequeueN(ctx, 1)
	require.NoError(t, err)
	require.Len(t, batch, 0)

	time.Sleep(3 * time.Second)

	promoted, err := q.ActivateDueRuns(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, promoted)
	require.NoError(t, q.ForceTick(ctx,
		"http"))

	var batch2 []domain.JobRun
	deadline := time.Now().Add(5 * time.Second)
	for len(batch2) == 0 && time.Now().Before(deadline) {
		require.NoError(t, q.ForceTick(ctx,
			"http"))

		var dequeueErr error
		batch2, dequeueErr = q.DequeueN(ctx, 1)
		require.Nil(t, dequeueErr)

		if len(batch2) == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if len(batch2) != 1 || batch2[0].ID != run.ID {
		var blocked bool
		var readyEvents, readyEmits, activeClaims int
		var latestRetryCleared bool
		require.Nil(t, env.
			DB.Pool.
			QueryRow(ctx,
				`
			SELECT
				strait_run_retry_blocked($1),
				(SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = $1 AND reason = 'retry_ready'),
				(SELECT COUNT(*) FROM strait_pgque_ready_events WHERE run_id = $1),
				(SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1),
				COALESCE((SELECT cleared FROM job_retries WHERE run_id = $1 ORDER BY id DESC LIMIT 1), FALSE)`,

				run.ID).Scan(
			&blocked, &readyEvents,
			&readyEmits, &activeClaims,

			&latestRetryCleared,
		))
		require.Failf(t, "test failure",

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
	require.NoError(t, q.Enqueue(ctx,
		run))

	exec, _ := newExecutor(t, env, srv.URL, 2, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for terminal completion")
		default:
		}
		got, err := st.GetRun(ctx, run.ID)
		require.NoError(t, err)

		if got.Status == domain.StatusCompleted {
			return
		}
		require.False(t, got.Status.
			IsTerminal())

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
		require.NoError(t, env.DB.
			Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = $1)`,

			idx,
		).Scan(&exists))
		assert.False(t, exists)

	}
}
