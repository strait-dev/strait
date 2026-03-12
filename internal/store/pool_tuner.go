package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const (
	defaultPoolTunerInterval       = 5 * time.Minute
	poolAdviceSampleConsistencyMin = 3
)

type poolProvider interface {
	Stat() *pgxpool.Stat
}

type poolTuningState struct {
	acquiredAtMaxStreak int
	idleAtMaxStreak     int
	lastWaitCount       int64
}

type poolSnapshot struct {
	Acquired  int32
	Idle      int32
	Total     int32
	WaitCount int64
}

type PoolTuner struct {
	pool     poolProvider
	logger   *slog.Logger
	interval time.Duration

	maxConns int32
	minConns int32

	acquiredGauge  metric.Int64Gauge
	idleGauge      metric.Int64Gauge
	totalGauge     metric.Int64Gauge
	waitCountGauge metric.Int64Gauge

	state poolTuningState
}

func NewPoolTuner(pool *pgxpool.Pool, logger *slog.Logger, maxConns, minConns int32) (*PoolTuner, error) {
	if pool == nil {
		return nil, fmt.Errorf("new pool tuner: pool is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	meter := otel.Meter("strait")
	acquiredGauge, err := meter.Int64Gauge(
		"strait_db_pool_acquired",
		metric.WithDescription("Current number of acquired database connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("new pool tuner: create acquired gauge: %w", err)
	}
	idleGauge, err := meter.Int64Gauge(
		"strait_db_pool_idle",
		metric.WithDescription("Current number of idle database connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("new pool tuner: create idle gauge: %w", err)
	}
	totalGauge, err := meter.Int64Gauge(
		"strait_db_pool_total",
		metric.WithDescription("Current total number of database connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("new pool tuner: create total gauge: %w", err)
	}
	waitCountGauge, err := meter.Int64Gauge(
		"strait_db_pool_wait_count",
		metric.WithDescription("Cumulative count of waits for an available DB connection"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("new pool tuner: create wait count gauge: %w", err)
	}

	return &PoolTuner{
		pool:           pool,
		logger:         logger,
		interval:       defaultPoolTunerInterval,
		maxConns:       maxConns,
		minConns:       minConns,
		acquiredGauge:  acquiredGauge,
		idleGauge:      idleGauge,
		totalGauge:     totalGauge,
		waitCountGauge: waitCountGauge,
		state: poolTuningState{
			lastWaitCount: -1,
		},
	}, nil
}

func (t *PoolTuner) Run(ctx context.Context) error {
	t.sample(ctx)

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.sample(ctx)
		}
	}
}

func (t *PoolTuner) sample(ctx context.Context) {
	stat := t.pool.Stat()
	snapshot := poolSnapshot{
		Acquired:  stat.AcquiredConns(),
		Idle:      stat.IdleConns(),
		Total:     stat.TotalConns(),
		WaitCount: stat.EmptyAcquireCount(),
	}

	t.acquiredGauge.Record(ctx, int64(snapshot.Acquired))
	t.idleGauge.Record(ctx, int64(snapshot.Idle))
	t.totalGauge.Record(ctx, int64(snapshot.Total))
	t.waitCountGauge.Record(ctx, snapshot.WaitCount)

	recommendations := evaluatePoolRecommendations(snapshot, t.maxConns, t.minConns, &t.state)
	for _, recommendation := range recommendations {
		t.logger.Warn("connection pool tuning recommendation",
			"recommendation", recommendation,
			"acquired", snapshot.Acquired,
			"idle", snapshot.Idle,
			"total", snapshot.Total,
			"wait_count", snapshot.WaitCount,
			"db_max_conns", t.maxConns,
			"db_min_conns", t.minConns,
		)
	}
}

func evaluatePoolRecommendations(snapshot poolSnapshot, maxConns, minConns int32, state *poolTuningState) []string {
	recommendations := make([]string, 0, 3)

	if maxConns > 0 && snapshot.Acquired >= maxConns {
		state.acquiredAtMaxStreak++
		if state.acquiredAtMaxStreak == poolAdviceSampleConsistencyMin {
			recommendations = append(recommendations, "Consider increasing DB_MAX_CONNS")
		}
	} else {
		state.acquiredAtMaxStreak = 0
	}

	if minConns > 0 && snapshot.Idle >= minConns {
		state.idleAtMaxStreak++
		if state.idleAtMaxStreak == poolAdviceSampleConsistencyMin {
			recommendations = append(recommendations, "Consider decreasing DB_MIN_CONNS")
		}
	} else {
		state.idleAtMaxStreak = 0
	}

	if state.lastWaitCount >= 0 && snapshot.WaitCount > state.lastWaitCount {
		recommendations = append(recommendations, "Connection pool under pressure, check query performance")
	}
	state.lastWaitCount = snapshot.WaitCount

	return recommendations
}
