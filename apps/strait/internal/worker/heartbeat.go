package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type HeartbeatManager struct {
	store               HeartbeatStore
	interval            time.Duration
	active              sync.Map
	registeredAt        sync.Map
	now                 func() time.Time
	consecutiveFailures int
}

func NewHeartbeatManager(s HeartbeatStore, interval time.Duration) *HeartbeatManager {
	return &HeartbeatManager{
		store:    s,
		interval: interval,
		now:      time.Now,
	}
}

type HeartbeatSender = HeartbeatManager

func (h *HeartbeatManager) Register(runID string) {
	h.active.Store(runID, struct{}{})
	h.registeredAt.Store(runID, h.now())
}

func (h *HeartbeatManager) Deregister(runID string) {
	h.active.Delete(runID)
	h.registeredAt.Delete(runID)
}

func (h *HeartbeatManager) ActiveCount() int {
	count := 0
	h.active.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (h *HeartbeatManager) Run(ctx context.Context, runIDs ...string) {
	for _, runID := range runIDs {
		h.Register(runID)
	}
	defer func() {
		for _, runID := range runIDs {
			h.Deregister(runID)
		}
	}()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.flush(ctx)
		}
	}
}

func (h *HeartbeatManager) flush(ctx context.Context) {
	ids := h.collectActiveIDs()
	if len(ids) == 0 {
		return
	}

	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	err := h.store.BatchUpdateHeartbeat(flushCtx, ids)
	elapsed := time.Since(start).Seconds()
	h.recordOldestLag(ctx, ids)

	if heartbeatFlushDuration != nil {
		heartbeatFlushDuration.Record(ctx, elapsed)
	}

	if err != nil {
		h.consecutiveFailures++
		if heartbeatFlushErrors != nil {
			heartbeatFlushErrors.Add(ctx, 1)
		}
		if h.consecutiveFailures >= 3 {
			slog.Warn("heartbeat flush failing repeatedly",
				"consecutive_failures", h.consecutiveFailures,
				"run_count", len(ids),
				"error", err,
			)
		}
		return
	}
	h.consecutiveFailures = 0
}

func (h *HeartbeatManager) collectActiveIDs() []string {
	ids := make([]string, 0, 16)
	h.active.Range(func(key, _ any) bool {
		if runID, ok := key.(string); ok {
			ids = append(ids, runID)
		}
		return true
	})
	return ids
}
