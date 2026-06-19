package scheduler

import (
	"context"
	"log/slog"
	"slices"
	"sync"
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
	now            func() time.Time
	retryMu        sync.Mutex
	retryHours     []time.Time
}

// statsAggregatorLockID is the pg_advisory_lock key for single-leader aggregation.
const statsAggregatorLockID int64 = 0x5374726169745361 // "StraitSa" as int64

// NewStatsAggregator creates a new stats aggregator.
func NewStatsAggregator(s StatsAggregatorStore) *StatsAggregator {
	return &StatsAggregator{
		store:  s,
		logger: slog.Default(),
		now:    time.Now,
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
		a.runCycle(loopCtx)
	})
	loop.Run(ctx)
}

func (a *StatsAggregator) runCycle(ctx context.Context) {
	acquired, err := runWithOptionalAdvisoryLock(ctx, a.advisoryLocker, statsAggregatorLockID, a.runLocked)
	if err != nil {
		a.logger.Error("stats aggregator advisory lock cycle failed", "error", err)
		return
	}
	if !acquired {
		a.logger.Debug("stats aggregator lock held by another instance, skipping")
	}
}

func (a *StatsAggregator) runLocked(ctx context.Context) error {
	previousHour := a.now().Add(-time.Hour).Truncate(time.Hour)
	failed := make([]time.Time, 0)
	for _, hour := range a.hoursToAggregate(previousHour) {
		if err := a.store.AggregateHourlyStats(ctx, hour); err != nil {
			a.logger.Error("failed to aggregate hourly stats", "hour", hour, "error", err)
			failed = append(failed, hour)
			continue
		}
		a.logger.Info("aggregated hourly stats", "hour", hour)

		if err := a.store.AggregateCostStatsHourly(ctx, hour); err != nil {
			a.logger.Error("failed to aggregate cost stats hourly", "hour", hour, "error", err)
			failed = append(failed, hour)
			continue
		}
		a.logger.Info("aggregated cost stats hourly", "hour", hour)
	}
	a.setRetryHours(failed)
	return nil
}

func (a *StatsAggregator) hoursToAggregate(previousHour time.Time) []time.Time {
	a.retryMu.Lock()
	defer a.retryMu.Unlock()

	hours := slices.Clone(a.retryHours)
	if !slices.ContainsFunc(hours, func(hour time.Time) bool { return hour.Equal(previousHour) }) {
		hours = append(hours, previousHour)
	}
	return hours
}

func (a *StatsAggregator) setRetryHours(hours []time.Time) {
	a.retryMu.Lock()
	defer a.retryMu.Unlock()
	a.retryHours = slices.Clone(hours)
}
