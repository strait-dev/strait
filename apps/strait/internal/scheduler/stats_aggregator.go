package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// StatsAggregatorStore is the subset of store operations needed by StatsAggregator.
type StatsAggregatorStore interface {
	AggregateHourlyStats(ctx context.Context, hour time.Time) error
	AggregateCostStatsHourly(ctx context.Context, hour time.Time) error
}

// StatsAggregator periodically materializes hourly run statistics.
type StatsAggregator struct {
	store          StatsAggregatorStore
	advisoryLocker AdvisoryLocker
	logger         *slog.Logger
}

// statsAggregatorLockID is the pg_advisory_lock key for single-leader aggregation.
const statsAggregatorLockID int64 = 0x5374726169745361 // "StraitSa" as int64

// NewStatsAggregator creates a new stats aggregator.
func NewStatsAggregator(s StatsAggregatorStore) *StatsAggregator {
	return &StatsAggregator{
		store:  s,
		logger: slog.Default(),
	}
}

// WithAdvisoryLocker enables distributed single-leader aggregation.
func (a *StatsAggregator) WithAdvisoryLocker(locker AdvisoryLocker) *StatsAggregator {
	a.advisoryLocker = locker
	return a
}

// Run starts the aggregation loop, running every hour.
func (a *StatsAggregator) Run(ctx context.Context) {
	loop := NewMaintenanceLoop("stats_aggregator", time.Hour, a.logger, func(loopCtx context.Context) {
		acquired, err := runWithOptionalAdvisoryLock(loopCtx, a.advisoryLocker, statsAggregatorLockID, a.runLocked)
		if err != nil {
			a.logger.Error("stats aggregator advisory lock cycle failed", "error", err)
			return
		}
		if !acquired {
			a.logger.Debug("stats aggregator lock held by another instance, skipping")
		}
	})
	loop.Run(ctx)
}

func (a *StatsAggregator) runLocked(ctx context.Context) error {
	previousHour := time.Now().Add(-time.Hour).Truncate(time.Hour)
	if err := a.store.AggregateHourlyStats(ctx, previousHour); err != nil {
		a.logger.Error("failed to aggregate hourly stats", "hour", previousHour, "error", err)
		return nil
	}
	a.logger.Info("aggregated hourly stats", "hour", previousHour)

	if err := a.store.AggregateCostStatsHourly(ctx, previousHour); err != nil {
		a.logger.Error("failed to aggregate cost stats hourly", "hour", previousHour, "error", err)
		return nil
	}
	a.logger.Info("aggregated cost stats hourly", "hour", previousHour)
	return nil
}
