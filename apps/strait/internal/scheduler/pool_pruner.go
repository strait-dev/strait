package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/compute"
)

// PoolPruner periodically removes expired machines from the warm pool.
type PoolPruner struct {
	pool     *compute.MachinePool
	runtime  compute.ContainerRuntime
	interval time.Duration
	ttl      time.Duration
	logger   *slog.Logger
}

// NewPoolPruner creates a new pool pruner.
func NewPoolPruner(pool *compute.MachinePool, runtime compute.ContainerRuntime, interval, ttl time.Duration) *PoolPruner {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &PoolPruner{
		pool:     pool,
		runtime:  runtime,
		interval: interval,
		ttl:      ttl,
		logger:   slog.Default(),
	}
}

// Run starts the prune loop. Blocks until ctx is canceled.
func (p *PoolPruner) Run(ctx context.Context) {
	if p.pool == nil {
		return
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruned := p.pool.Prune(p.ttl, func(machineID string) error {
				if p.runtime != nil {
					return p.runtime.Destroy(ctx, machineID)
				}
				return nil
			})
			if pruned > 0 {
				p.logger.Info("pool pruner: removed expired machines",
					"count", pruned,
					"pool_size", p.pool.Size(),
				)
			}
		}
	}
}
