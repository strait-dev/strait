package worker

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"
)

type AdaptiveProbe func(ctx context.Context) (queueDepth int, utilization float64, err error)

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
	a.mu.Lock()
	defer a.mu.Unlock()

	current := a.current
	if queueDepth > current*2 && utilization > 0.80 {
		inc := max(int(math.Ceil(float64(current)*0.25)), 1)
		a.current = min(current+inc, a.max)
		a.idleChecks = 0
		return a.current
	}

	if queueDepth == 0 && utilization < 0.20 {
		a.idleChecks++
		if a.idleChecks >= 2 {
			dec := max(int(math.Ceil(float64(current)*0.25)), 1)
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
			queueDepth, utilization, err := probe(ctx)
			if err != nil {
				logger.Warn("adaptive concurrency probe failed", "error", err)
				continue
			}

			before := a.CurrentLimit()
			after := a.Observe(queueDepth, utilization)
			if after != before {
				logger.Info("adaptive concurrency updated", "previous", before, "current", after, "queue_depth", queueDepth, "utilization", utilization)
			}
		}
	}
}
