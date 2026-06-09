package worker

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type HeartbeatManager struct {
	store               HeartbeatStore
	interval            time.Duration
	activeMu            sync.RWMutex
	active              map[string]time.Time
	activeCount         atomic.Int64
	now                 func() time.Time
	consecutiveFailures int
}

func NewHeartbeatManager(s HeartbeatStore, interval time.Duration) *HeartbeatManager {
	return &HeartbeatManager{
		store:    s,
		interval: interval,
		active:   make(map[string]time.Time),
		now:      time.Now,
	}
}

type HeartbeatSender = HeartbeatManager

func (h *HeartbeatManager) Register(runID string) {
	now := h.now()
	h.activeMu.Lock()
	if _, loaded := h.active[runID]; !loaded {
		h.activeCount.Add(1)
	}
	h.active[runID] = now
	h.activeMu.Unlock()
}

func (h *HeartbeatManager) Deregister(runID string) {
	h.activeMu.Lock()
	if _, loaded := h.active[runID]; loaded {
		delete(h.active, runID)
		h.activeCount.Add(-1)
	}
	h.activeMu.Unlock()
}

func (h *HeartbeatManager) ActiveCount() int {
	return int(h.activeCount.Load())
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
	h.activeMu.RLock()
	capacity := len(h.active)
	ids := make([]string, 0, capacity)
	for runID := range h.active {
		ids = append(ids, runID)
	}
	h.activeMu.RUnlock()
	return ids
}
