package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// PollerStore is the subset of store operations needed by DelayedPoller.
type PollerStore interface {
	ActivateDueRuns(ctx context.Context, limit int) (int64, error)
}

type DelayedPoller struct {
	store    PollerStore
	logger   *slog.Logger
	interval time.Duration
}

// NewDelayedPoller creates a new delayed run poller.
func NewDelayedPoller(s PollerStore, logger *slog.Logger, interval time.Duration) *DelayedPoller {
	return &DelayedPoller{
		store:    s,
		logger:   logger,
		interval: interval,
	}
}

func (p *DelayedPoller) Run(ctx context.Context) {
	p.logger.Info("delayed poller started", "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("delayed poller stopping")
			return
		case <-ticker.C:
			activated, err := p.store.ActivateDueRuns(ctx, 1000)
			if err != nil {
				p.logger.Error("failed to activate due runs", "error", err)
				continue
			}
			if activated > 0 {
				p.logger.Info("activated delayed runs", "count", activated)
			}
		}
	}
}
