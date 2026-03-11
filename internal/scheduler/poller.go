package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"
)

// PollerStore is the subset of store operations needed by DelayedPoller.
type PollerStore interface {
	ListDueRuns(ctx context.Context) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
}

type DelayedPoller struct {
	store    PollerStore
	interval time.Duration
	metrics  *telemetry.Metrics
	lastTick atomic.Int64
}

// NewDelayedPoller creates a new delayed run poller.
func NewDelayedPoller(s PollerStore, interval time.Duration) *DelayedPoller {
	p := &DelayedPoller{
		store:    s,
		interval: interval,
	}
	p.lastTick.Store(time.Now().UnixNano())
	return p
}

func (p *DelayedPoller) WithMetrics(m *telemetry.Metrics) *DelayedPoller {
	p.metrics = m
	return p
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
			p.lastTick.Store(time.Now().UnixNano())
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
				if p.metrics != nil {
					p.metrics.PollerRunsQueued.Add(ctx, 1)
				}

				slog.Info("due run queued", "run_id", run.ID, "job_id", run.JobID)
			}
		}
	}
}

func (p *DelayedPoller) LastTick() time.Time {
	nanos := p.lastTick.Load()
	if nanos <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}
