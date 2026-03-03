package scheduler

import (
	"context"
	"log/slog"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
)

type Reaper struct {
	store          store.Store
	interval       time.Duration
	staleThreshold time.Duration
}

func NewReaper(s store.Store, interval, staleThreshold time.Duration) *Reaper {
	return &Reaper{
		store:          s,
		interval:       interval,
		staleThreshold: staleThreshold,
	}
}

func (r *Reaper) Run(ctx context.Context) {
	slog.Info("reaper started", "interval", r.interval, "stale_threshold", r.staleThreshold)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reaper stopping")
			return
		case <-ticker.C:
			r.reapStaleDequeued(ctx)
			r.reapStale(ctx)
			r.reapExpired(ctx)
		}
	}
}

func (r *Reaper) reapStale(ctx context.Context) {
	runs, err := r.store.ListStaleRuns(ctx, r.staleThreshold)
	if err != nil {
		slog.Error("failed to list stale runs", "error", err)
		return
	}

	for _, run := range runs {
		err := r.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCrashed, map[string]any{
			"finished_at": time.Now(),
			"error":       "heartbeat lost",
		})
		if err != nil {
			slog.Error("failed to crash stale run", "run_id", run.ID, "job_id", run.JobID, "error", err)
			continue
		}

		slog.Warn("stale run marked crashed", "run_id", run.ID, "job_id", run.JobID)
	}
}

func (r *Reaper) reapExpired(ctx context.Context) {
	runs, err := r.store.ListExpiredRuns(ctx)
	if err != nil {
		slog.Error("failed to list expired runs", "error", err)
		return
	}

	for _, run := range runs {
		err := r.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusExpired, map[string]any{
			"finished_at": time.Now(),
			"error":       "run expired",
		})
		if err != nil {
			slog.Error("failed to expire run", "run_id", run.ID, "job_id", run.JobID, "from_status", run.Status, "error", err)
			continue
		}

		slog.Warn("run marked expired", "run_id", run.ID, "job_id", run.JobID, "from_status", run.Status)
	}
}

func (r *Reaper) reapStaleDequeued(ctx context.Context) {
	runs, err := r.store.ListStaleDequeued(ctx, r.staleThreshold)
	if err != nil {
		slog.Error("failed to list stale dequeued runs", "error", err)
		return
	}

	for _, run := range runs {
		err := r.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, map[string]any{
			"started_at": nil,
		})
		if err != nil {
			slog.Error("failed to re-queue stale dequeued run", "run_id", run.ID, "job_id", run.JobID, "error", err)
			continue
		}

		slog.Warn("stale dequeued run re-queued", "run_id", run.ID, "job_id", run.JobID)
	}
}
