package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// CostEstimateRefresherStore defines the store operations needed by CostEstimateRefresher.
type CostEstimateRefresherStore interface {
	ListActiveJobIDs(ctx context.Context) ([]string, error)
	UpsertJobCostEstimate(ctx context.Context, jobID string) error
}

// CostEstimateRefresher periodically recomputes job cost estimates.
type CostEstimateRefresher struct {
	store    CostEstimateRefresherStore
	interval time.Duration
	logger   *slog.Logger
}

// NewCostEstimateRefresher creates a new cost estimate refresher.
func NewCostEstimateRefresher(s CostEstimateRefresherStore, interval time.Duration) *CostEstimateRefresher {
	if interval <= 0 {
		interval = time.Hour
	}
	return &CostEstimateRefresher{
		store:    s,
		interval: interval,
		logger:   slog.Default(),
	}
}

// Run starts the cost estimate refresh loop. Blocks until ctx is canceled.
func (r *CostEstimateRefresher) Run(ctx context.Context) {
	loop := NewMaintenanceLoop("cost_estimate_refresher", r.interval, r.logger, func(loopCtx context.Context) {
		r.refresh(loopCtx)
	})
	loop.Run(ctx)
}

func (r *CostEstimateRefresher) refresh(ctx context.Context) {
	jobIDs, err := r.store.ListActiveJobIDs(ctx)
	if err != nil {
		r.logger.Warn("cost estimate refresher: failed to list active jobs", "error", err)
		return
	}

	var updated int
	for _, jobID := range jobIDs {
		if err := r.store.UpsertJobCostEstimate(ctx, jobID); err != nil {
			r.logger.Warn("cost estimate refresher: failed to upsert estimate",
				"job_id", jobID, "error", err)
			continue
		}
		updated++
	}

	if updated > 0 {
		r.logger.Info("cost estimate refresher: completed",
			"jobs_updated", updated, "jobs_total", len(jobIDs))
	}
}
