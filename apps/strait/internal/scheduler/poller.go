package scheduler

import (
	"context"
	"log/slog"
	"time"
)

const (
	defaultDelayedPollerBatchLimit        = 1000
	defaultDelayedPollerMaxBatchesPerTick = 16
)

// PollerStore is the subset of store operations needed by DelayedPoller.
type PollerStore interface {
	ActivateDueRuns(ctx context.Context, limit int) (int64, error)
}

type DelayedPoller struct {
	store             PollerStore
	promoter          PollerStore
	logger            *slog.Logger
	interval          time.Duration
	batchLimit        int
	maxBatchesPerTick int
}

// NewDelayedPoller creates a new delayed run poller.
func NewDelayedPoller(s PollerStore, logger *slog.Logger, interval time.Duration) *DelayedPoller {
	if interval <= 0 {
		interval = time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DelayedPoller{
		store:             s,
		logger:            logger,
		interval:          interval,
		batchLimit:        defaultDelayedPollerBatchLimit,
		maxBatchesPerTick: defaultDelayedPollerMaxBatchesPerTick,
	}
}

// WithPromoter overrides the store used for delayed-run activation. PgQue
// provides the atomic promote+emit path used by production wiring.
func (p *DelayedPoller) WithPromoter(promoter PollerStore) *DelayedPoller {
	if promoter != nil {
		p.promoter = promoter
	}
	return p
}

// WithBatchLimit sets the maximum delayed runs promoted by one store call.
func (p *DelayedPoller) WithBatchLimit(limit int) *DelayedPoller {
	if limit > 0 {
		p.batchLimit = limit
	}
	return p
}

// WithMaxBatchesPerTick bounds catch-up work so one large backlog cannot
// monopolize the scheduler loop forever.
func (p *DelayedPoller) WithMaxBatchesPerTick(limit int) *DelayedPoller {
	if limit > 0 {
		p.maxBatchesPerTick = limit
	}
	return p
}

func (p *DelayedPoller) Run(ctx context.Context) {
	p.logger.Info("delayed poller started", "interval", p.interval, "batch_limit", p.batchLimit, "max_batches_per_tick", p.maxBatchesPerTick)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("delayed poller stopping")
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, p.interval, func() {
				p.poll(ctx)
			})
		}
	}
}

func (p *DelayedPoller) poll(ctx context.Context) {
	var total int64
	promoter := p.store
	if p.promoter != nil {
		promoter = p.promoter
	}
	for range p.maxBatchesPerTick {
		if err := ctx.Err(); err != nil {
			return
		}
		activated, err := promoter.ActivateDueRuns(ctx, p.batchLimit)
		if err != nil {
			p.logger.Error("failed to activate due runs", "error", err, "activated_before_error", total)
			return
		}
		total += activated
		if activated < int64(p.batchLimit) {
			break
		}
	}
	if total > 0 {
		p.logger.Info("activated delayed runs", "count", total)
	}
}
