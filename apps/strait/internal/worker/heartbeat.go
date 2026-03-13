package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type HeartbeatStore interface {
	UpdateHeartbeat(ctx context.Context, id string) error
	BatchUpdateHeartbeat(ctx context.Context, ids []string) error
}

type heartbeatLegacyStore interface {
	UpdateHeartbeat(ctx context.Context, id string) error
}

type heartbeatStoreAdapter struct {
	store heartbeatLegacyStore
}

func (a heartbeatStoreAdapter) UpdateHeartbeat(ctx context.Context, id string) error {
	return a.store.UpdateHeartbeat(ctx, id)
}

func (a heartbeatStoreAdapter) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := a.store.UpdateHeartbeat(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

type HeartbeatManager struct {
	store    HeartbeatStore
	interval time.Duration
	active   sync.Map
	now      func() time.Time
}

func NewHeartbeatManager(s HeartbeatStore, interval time.Duration) *HeartbeatManager {
	return &HeartbeatManager{
		store:    s,
		interval: interval,
		now:      time.Now,
	}
}

func NewHeartbeatSender(s heartbeatLegacyStore, interval time.Duration) *HeartbeatManager {
	if store, ok := s.(HeartbeatStore); ok {
		return NewHeartbeatManager(store, interval)
	}
	return NewHeartbeatManager(heartbeatStoreAdapter{store: s}, interval)
}

type HeartbeatSender = HeartbeatManager

func (h *HeartbeatManager) Register(runID string) {
	h.active.Store(runID, struct{}{})
}

func (h *HeartbeatManager) Deregister(runID string) {
	h.active.Delete(runID)
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
	ids := make([]string, 0, h.ActiveCount())
	h.active.Range(func(key, _ any) bool {
		runID, ok := key.(string)
		if ok {
			ids = append(ids, runID)
		}
		return true
	})

	if len(ids) == 0 {
		return
	}

	if err := h.store.BatchUpdateHeartbeat(ctx, ids); err != nil {
		slog.Warn("heartbeat batch update failed",
			"run_count", len(ids),
			"at", h.now(),
			"error", err,
		)
	}
}
