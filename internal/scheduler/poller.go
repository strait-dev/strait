package scheduler

import (
	"context"
	"log/slog"
	"time"

	"orchestrator/internal/domain"
)

// PollerStore is the subset of store operations needed by DelayedPoller.
type PollerStore interface {
	ListDueRuns(ctx context.Context) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
}

type DelayedPoller struct {
	store    PollerStore
	interval time.Duration
}

// NewDelayedPoller creates a new delayed run poller.
func NewDelayedPoller(s PollerStore, interval time.Duration) *DelayedPoller {
	return &DelayedPoller{
		store:    s,
		interval: interval,
	}
}

func (p *DelayedPoller) Run(ctx context.Context) {
	slog.Info("delayed poller started", "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("delayed poller stopping")
			return
		case <-ticker.C:
			runs, err := p.store.ListDueRuns(ctx)
			if err != nil {
				slog.Error("failed to list due runs", "error", err)
				continue
			}

			for _, run := range runs {
				err := p.store.UpdateRunStatus(ctx, run.ID, domain.StatusDelayed, domain.StatusQueued, nil)
				if err != nil {
					slog.Error("failed to queue due run", "run_id", run.ID, "job_id", run.JobID, "error", err)
					continue
				}

				slog.Info("due run queued", "run_id", run.ID, "job_id", run.JobID)
			}
		}
	}
}
