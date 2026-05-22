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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, map[string]any{
		"error":       "transient",
		"error_class": "transient",
	})
	if err != nil {
		t.Fatalf("SnoozeRunWithLock() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("status = %s, want queued", got.Status)
	}
}

// TestSnoozeRunWithLock_RunNotFound verifies a missing row surfaces
// ErrRunNotFound rather than a generic error.
func TestSnoozeRunWithLock_RunNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.SnoozeRunWithLock(ctx, newID(), domain.StatusDequeued, domain.StatusQueued, nil)
	if !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound", err)
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
	if !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("err = %v, want ErrRunConflict", err)
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	// Start a competing tx and grab the row lock. Hold it until the
	// SnoozeRunWithLock call returns.
	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM job_runs WHERE id = $1 FOR UPDATE`, run.ID).Scan(&locked); err != nil {
		t.Fatalf("competing FOR UPDATE error = %v", err)
	}

	snoozeErr := q.SnoozeRunWithLock(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, nil)
	if !errors.Is(snoozeErr, store.ErrRunLocked) {
		t.Fatalf("err = %v, want ErrRunLocked", snoozeErr)
	}

	// Status must NOT have transitioned while the lock was held.
	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusDequeued {
		t.Fatalf("status = %s, want dequeued (transition leaked through SKIP LOCKED)", got.Status)
	}
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
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

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

	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (locked=%d conflict=%d other=%d)", winners, locked, conflict, other)
	}
	if other != 0 {
		t.Fatalf("got %d genuine errors; SKIP LOCKED must never leak DB errors to callers", other)
	}
	if locked+conflict != n-1 {
		t.Fatalf("locked+conflict = %d, want %d", locked+conflict, n-1)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusQueued {
		t.Fatalf("final status = %s, want queued", got.Status)
	}
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
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
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
			if !errors.Is(err, store.ErrRunLocked) && !errors.Is(err, store.ErrRunConflict) {
				t.Errorf("run %s: snooze err = %v", run.ID, err)
			}
		})
		wg.Go(func() {
			<-start
			err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil)
			if err == nil {
				completionWins.Add(1)
				return
			}
			if !errors.Is(err, store.ErrRunConflict) {
				t.Errorf("run %s: complete err = %v", run.ID, err)
			}
		})
	}
	close(start)
	wg.Wait()

	if snoozeWins.Load()+completionWins.Load() != int32(n) {
		t.Fatalf("snoozeWins=%d completionWins=%d total=%d, want %d",
			snoozeWins.Load(), completionWins.Load(), snoozeWins.Load()+completionWins.Load(), n)
	}

	for _, run := range runs {
		got, err := q.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun(%s) error = %v", run.ID, err)
		}
		if got.Status != domain.StatusQueued && got.Status != domain.StatusExecuting {
			t.Fatalf("run %s final status = %s, want queued or executing", run.ID, got.Status)
		}
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
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
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

	if deadlocks.Load() != 0 {
		t.Fatalf("deadlocks observed: %d", deadlocks.Load())
	}

	for _, run := range runs {
		got, err := q.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun(%s) error = %v", run.ID, err)
		}
		if got.Status != domain.StatusDequeued && got.Status != domain.StatusQueued {
			t.Fatalf("run %s ended in unexpected status %s", run.ID, got.Status)
		}
	}
}
