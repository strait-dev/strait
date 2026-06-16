package worker

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"
)

type AdaptiveProbe func(ctx context.Context) (queueDepth int, utilization float64, dbPressure bool, err error)

type AdaptiveConcurrency struct {
	min     int
	max     int
	current int

	idleChecks int

	mu sync.Mutex
}

func NewAdaptiveConcurrency(minLimit, maxLimit, initial int) *AdaptiveConcurrency {
	if minLimit < 1 {
		minLimit = 1
	}
	if maxLimit < minLimit {
		maxLimit = minLimit
	}
	if initial < minLimit {
		initial = minLimit
	}
	if initial > maxLimit {
		initial = maxLimit
	}

	return &AdaptiveConcurrency{
		min:     minLimit,
		max:     maxLimit,
		current: initial,
	}
}

func (a *AdaptiveConcurrency) CurrentLimit() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.current
}

func (a *AdaptiveConcurrency) Observe(queueDepth int, utilization float64) int {
	return a.ObserveWithPressure(queueDepth, utilization, false)
}

func (a *AdaptiveConcurrency) ObserveWithPressure(queueDepth int, utilization float64, dbPressure bool) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	current := a.current
	if dbPressure {
		dec := max(int(math.Ceil(float64(current)*0.33)), 1)
		a.current = max(current-dec, a.min)
		a.idleChecks = 0
		return a.current
	}

	// Scale up: tiered by queue depth severity.
	if queueDepth > current*2 && utilization > 0.70 {
		var factor float64
		switch {
		case queueDepth > 1000:
			factor = 1.0 // Double on deep queue
		case queueDepth > 100:
			factor = 0.50 // 50% increase on moderate queue
		default:
			factor = 0.25 // 25% increase on mild backlog
		}
		inc := max(int(math.Ceil(float64(current)*factor)), 1)
		a.current = min(current+inc, a.max)
		a.idleChecks = 0
		return a.current
	}

	// Scale down only when real work is running at low utilization. A fully idle
	// pool is cold-start capacity, not evidence that workers should be shed.
	if queueDepth == 0 && utilization > 0 && utilization < 0.20 {
		a.idleChecks++
		if a.idleChecks >= 2 {
			dec := max(int(math.Ceil(float64(current)*0.33)), 1)
			a.current = max(current-dec, a.min)
			a.idleChecks = 0
		}
		return a.current
	}

	a.idleChecks = 0
	return a.current
}

func (a *AdaptiveConcurrency) Run(ctx context.Context, interval time.Duration, probe AdaptiveProbe, logger *slog.Logger) {
	if probe == nil {
		return
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queueDepth, utilization, dbPressure, err := probe(ctx)
			if err != nil {
				logger.Warn("adaptive concurrency probe failed", "error", err)
				continue
			}

			before := a.CurrentLimit()
			after := a.ObserveWithPressure(queueDepth, utilization, dbPressure)
			if after != before {
				logger.Info("adaptive concurrency updated", "previous", before, "current", after, "queue_depth", queueDepth, "utilization", utilization, "db_pressure", dbPressure)
			}
		}
	}
}
