package telemetry

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// PoolSampler periodically reads pgxpool.Pool.Stat() and records the four
// connection-pool gauges (acquired, idle, total, max). It runs in a goroutine
// and stops when ctx is cancelled.
type PoolSampler struct {
	pool     *pgxpool.Pool
	interval time.Duration
	logger   *slog.Logger
	acquired metric.Int64Gauge
	idle     metric.Int64Gauge
	total    metric.Int64Gauge
	maxConns metric.Int64Gauge
}

// NewPoolSampler creates a sampler that records DB pool metrics every interval.
// A zero or negative interval defaults to 15s.
func NewPoolSampler(pool *pgxpool.Pool, interval time.Duration, logger *slog.Logger) (*PoolSampler, error) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	meter := otel.Meter("strait/db_pool")

	acquired, err := meter.Int64Gauge("strait.db.pool_acquired_conns",
		metric.WithDescription("Connections currently acquired from the pool"))
	if err != nil {
		return nil, err
	}
	idle, err := meter.Int64Gauge("strait.db.pool_idle_conns",
		metric.WithDescription("Connections currently idle in the pool"))
	if err != nil {
		return nil, err
	}
	total, err := meter.Int64Gauge("strait.db.pool_total_conns",
		metric.WithDescription("Total connections in the pool (acquired + idle)"))
	if err != nil {
		return nil, err
	}
	maxConns, err := meter.Int64Gauge("strait.db.pool_max_conns",
		metric.WithDescription("Maximum pool size"))
	if err != nil {
		return nil, err
	}

	return &PoolSampler{
		pool: pool, interval: interval, logger: logger,
		acquired: acquired, idle: idle, total: total, maxConns: maxConns,
	}, nil
}

// Run samples pool stats on every tick until ctx is cancelled.
func (s *PoolSampler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.logger.Warn("pool sampler panic recovered", "panic", r)
					}
				}()
				stat := s.pool.Stat()
				s.acquired.Record(ctx, int64(stat.AcquiredConns()))
				s.idle.Record(ctx, int64(stat.IdleConns()))
				s.total.Record(ctx, int64(stat.TotalConns()))
				s.maxConns.Record(ctx, int64(stat.MaxConns()))
			}()
		}
	}
}
