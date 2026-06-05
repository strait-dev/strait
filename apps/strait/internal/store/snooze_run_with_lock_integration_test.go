//go:build integration

package store_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnoozeRunWithLock_HappyPath verifies a clean dequeued -> queued
// transition on a row that nobody else holds.
func TestSnoozeRunWithLock_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-happy")
	run := baseRun(job, newID())
	run.Status = domain.StatusDequeued
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, map[string]any{
		"error":       "transient",
		"error_class": "transient",
	})
	require.NoError(t, err)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		got.Status,
	)

}

func TestSnoozeRunWithLock_RequeuesActiveClaimOverlay(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-active-claim")
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx, run))

	var beforeGeneration int64
	require.NoError(t, testDB.Pool.QueryRow(ctx, `
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,
		run.ID,
	).Scan(&beforeGeneration))
	started := time.Now().UTC().Add(-10 * time.Minute)
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		VALUES ($1, $2, 1, $3)`,
		run.ID, beforeGeneration, started,
	)
	require.NoError(t, err)

	err = q.SnoozeRunWithLock(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
		"error":        nil,
		"error_class":  nil,
		"started_at":   nil,
		"finished_at":  nil,
		"heartbeat_at": nil,
	})
	require.NoError(t, err)

	var stateStatus domain.RunStatus
	var afterGeneration int64
	var activeClaims int
	require.NoError(t, testDB.Pool.QueryRow(ctx, `
		SELECT s.status, s.ready_generation,
		       (SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = s.run_id)
		FROM job_run_state s
		WHERE s.run_id = $1`,
		run.ID,
	).Scan(&stateStatus, &afterGeneration, &activeClaims))
	require.Equal(t, domain.StatusQueued, stateStatus)
	require.Equal(t, beforeGeneration+1, afterGeneration)
	require.Equal(t, 1, activeClaims)
	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusQueued, got.Status)
}

// TestSnoozeRunWithLock_RunNotFound verifies a missing row surfaces
// ErrRunNotFound rather than a generic error.
func TestSnoozeRunWithLock_RunNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.SnoozeRunWithLock(ctx, newID(), domain.StatusDequeued, domain.StatusQueued, nil)
	require.True(t, errors.Is(err, store.
		ErrRunNotFound,
	))

}

// TestSnoozeRunWithLock_StatusConflictReturnsConflict verifies a row that
// has moved past the expected `from` status surfaces ErrRunConflict so the
// worker can no-op cleanly.
func TestSnoozeRunWithLock_StatusConflictReturnsConflict(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-conflict")
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
	require.True(t, errors.Is(err, store.
		ErrRunConflict,
	))

}

// TestSnoozeRunWithLock_LockedRowReturnsErrRunLocked verifies that when
// another transaction holds the row, SnoozeRunWithLock returns ErrRunLocked
// rather than blocking (the SELECT uses SKIP LOCKED) or erroring loudly.
func TestSnoozeRunWithLock_LockedRowReturnsErrRunLocked(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-locked")
	run := baseRun(job, newID())
	run.Status = domain.StatusDequeued
	require.NoError(t, q.CreateRun(ctx,
		run))

	// Start a competing tx and grab the row lock. Hold it until the
	// SnoozeRunWithLock call returns.
	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT run_id FROM job_run_state WHERE run_id = $1 FOR UPDATE`,

		run.ID).Scan(&locked))

	snoozeErr := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
	require.True(t, errors.Is(snoozeErr,
		store.ErrRunLocked,
	))

	// Status must NOT have transitioned while the lock was held.
	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDequeued,
		got.
			Status)

}

// TestSnoozeRunWithLock_ConcurrentSnoozesExactlyOneWinner is the regression
// test for STR-528: N goroutines all try to snooze the same row, exactly one
// succeeds, the rest return ErrRunLocked or ErrRunConflict, and the final
// row status is queued. No genuine errors should surface.
func TestSnoozeRunWithLock_ConcurrentSnoozesExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-concurrent")
	run := baseRun(job, newID())
	run.Status = domain.StatusDequeued
	require.NoError(t, q.CreateRun(ctx,
		run))

	const n = 25
	results := make(chan error, n)
	start := make(chan struct{})
	var wg conc.WaitGroup

	for range n {
		wg.Go(func() {
			<-start
			results <- q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
		})
	}
	close(start)
	wg.Wait()
	close(results)

	var winners, locked, conflict, other int
	for err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, store.ErrRunLocked):
			locked++
		case errors.Is(err, store.ErrRunConflict):
			conflict++
		default:
			other++
			t.Logf("unexpected error: %v", err)
		}
	}
	require.EqualValues(t, 1, winners)
	require.EqualValues(t, 0, other)
	require.Equal(t, n-1,

		locked+
			conflict)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		got.Status,
	)

}

// TestSnoozeRunWithLock_RacesWithCompletion simulates the original
// production hazard: while the dispatcher is about to snooze a Dequeued
// row, another tx flips the row to Executing (or Completed). The snooze
// must observe ErrRunConflict and the terminal state must win.
func TestSnoozeRunWithLock_RacesWithCompletion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-race-completion")

	const n = 10
	runs := make([]*domain.JobRun, n)
	for i := range n {
		run := baseRun(job, newID())
		run.Status = domain.StatusDequeued
		require.NoError(t, q.CreateRun(ctx,
			run))

		runs[i] = run
	}

	var snoozeWins, completionWins atomic.Int32
	start := make(chan struct{})
	var wg conc.WaitGroup

	for _, run := range runs {
		wg.Go(func() {
			<-start
			err := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
			if err == nil {
				snoozeWins.Add(1)
				return
			}
			assert.False(t, !errors.Is(err, store.
				ErrRunLocked,
			) &&
				!errors.Is(err,
					store.ErrRunConflict,
				))

		})
		wg.Go(func() {
			<-start
			err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil)
			if err == nil {
				completionWins.Add(1)
				return
			}
			assert.True(t, errors.Is(err, store.
				ErrRunConflict,
			))

		})
	}
	close(start)
	wg.Wait()
	require.Equal(t, int32(
		n,
	), snoozeWins.
		Load()+
		completionWins.
			Load())

	for _, run := range runs {
		got, err := q.GetRun(ctx, run.ID)
		require.NoError(t, err)
		require.False(t, got.Status !=
			domain.
				StatusQueued &&
			got.Status != domain.
				StatusExecuting,
		)

	}
}

// TestSnoozeRunWithLock_HighContention_NoDeadlock runs many goroutines
// snoozing a small set of rows in random order. The contract: no goroutine
// returns a deadlock / serialization error, and every row ends in a state
// that the snooze path could legitimately produce.
func TestSnoozeRunWithLock_HighContention_NoDeadlock(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-snooze-lock-high-contention")

	const numRuns = 5
	runs := make([]*domain.JobRun, numRuns)
	for i := range numRuns {
		run := baseRun(job, newID())
		run.Status = domain.StatusDequeued
		require.NoError(t, q.CreateRun(ctx,
			run))

		runs[i] = run
	}

	const workers = 50
	deadline := time.Now().Add(2 * time.Second)
	var deadlocks atomic.Int32
	var wg conc.WaitGroup

	for w := range workers {
		wg.Go(func() {
			for time.Now().Before(deadline) {
				run := runs[(w*7+int(time.Now().UnixNano()))%numRuns]
				_ = q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
				// Flip one back so contention persists.
				_ = q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, deadlocks.
		Load())

	for _, run := range runs {
		got, err := q.GetRun(ctx, run.ID)
		require.NoError(t, err)
		require.False(t, got.Status !=
			domain.
				StatusDequeued &&
			got.Status !=
				domain.StatusQueued,
		)

	}
}
