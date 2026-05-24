//go:build integration

package scheduler_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/store"
)

func intCreateDelayedRun(t *testing.T, ctx context.Context, st *store.Queries, job *domain.Job, scheduledAt time.Time) *domain.JobRun {
	t.Helper()
	run := &domain.JobRun{
		ID:          intNewID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusDelayed,
		ScheduledAt: &scheduledAt,
		TriggeredBy: domain.TriggerManual,
	}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	return run
}

func intCountRunsByStatus(t *testing.T, ctx context.Context, st *store.Queries, jobID string, status domain.RunStatus) int {
	t.Helper()
	var count int
	if err := getTestDB(t).Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND status = $2`,
		jobID, status,
	).Scan(&count); err != nil {
		t.Fatalf("count runs by status: %v", err)
	}
	return count
}

func intWaitForRunStatusCount(t *testing.T, ctx context.Context, st *store.Queries, jobID string, status domain.RunStatus, want int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			got := intCountRunsByStatus(t, ctx, st, jobID, status)
			t.Fatalf("timed out waiting for %d %s runs, got %d", want, status, got)
		case <-tick.C:
			if got := intCountRunsByStatus(t, ctx, st, jobID, status); got == want {
				return
			}
		}
	}
}

func TestIntegration_DelayedPoller_ImmediateActivation(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-poller-immediate")
	intCreateDelayedRun(t, ctx, st, job, time.Now().Add(-time.Minute))

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		scheduler.NewDelayedPoller(st, slog.Default(), time.Hour).Run(runCtx)
		close(done)
	}()

	intWaitForRunStatusCount(t, ctx, st, job.ID, domain.StatusQueued, 1)
	cancel()
	<-done
}

func TestIntegration_DelayedPoller_DrainsMultipleBatchesPerTick(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-poller-backlog")
	for range 7 {
		intCreateDelayedRun(t, ctx, st, job, time.Now().Add(-time.Minute))
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		scheduler.NewDelayedPoller(st, slog.Default(), time.Hour).
			WithBatchLimit(3).
			WithMaxBatchesPerTick(3).
			Run(runCtx)
		close(done)
	}()

	intWaitForRunStatusCount(t, ctx, st, job.ID, domain.StatusQueued, 7)
	cancel()
	<-done
}

func TestIntegration_DelayedPoller_RespectsPerTickBound(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-poller-bound")
	for range 8 {
		intCreateDelayedRun(t, ctx, st, job, time.Now().Add(-time.Minute))
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		scheduler.NewDelayedPoller(st, slog.Default(), time.Hour).
			WithBatchLimit(3).
			WithMaxBatchesPerTick(2).
			Run(runCtx)
		close(done)
	}()

	intWaitForRunStatusCount(t, ctx, st, job.ID, domain.StatusQueued, 6)
	if got := intCountRunsByStatus(t, ctx, st, job.ID, domain.StatusDelayed); got != 2 {
		t.Fatalf("delayed runs after bounded tick = %d, want 2", got)
	}
	cancel()
	<-done
}

func TestIntegration_DelayedPoller_KeepsFutureRunsDelayed(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-poller-future")
	for range 2 {
		intCreateDelayedRun(t, ctx, st, job, time.Now().Add(-time.Minute))
	}
	for range 3 {
		intCreateDelayedRun(t, ctx, st, job, time.Now().Add(time.Hour))
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		scheduler.NewDelayedPoller(st, slog.Default(), time.Hour).
			WithBatchLimit(10).
			Run(runCtx)
		close(done)
	}()

	intWaitForRunStatusCount(t, ctx, st, job.ID, domain.StatusQueued, 2)
	if got := intCountRunsByStatus(t, ctx, st, job.ID, domain.StatusDelayed); got != 3 {
		t.Fatalf("future delayed runs = %d, want 3", got)
	}
	cancel()
	<-done
}

func TestIntegration_DelayedPoller_ConcurrentPollersShareBacklog(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	job := intCreateJob(t, ctx, st, "proj-poller-concurrent")
	for range 10 {
		intCreateDelayedRun(t, ctx, st, job, time.Now().Add(-time.Minute))
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() {
		scheduler.NewDelayedPoller(st, slog.Default(), time.Hour).
			WithBatchLimit(3).
			WithMaxBatchesPerTick(2).
			Run(runCtx)
		close(doneA)
	}()
	go func() {
		scheduler.NewDelayedPoller(st, slog.Default(), time.Hour).
			WithBatchLimit(3).
			WithMaxBatchesPerTick(2).
			Run(runCtx)
		close(doneB)
	}()

	intWaitForRunStatusCount(t, ctx, st, job.ID, domain.StatusQueued, 10)
	cancel()
	<-doneA
	<-doneB
}
