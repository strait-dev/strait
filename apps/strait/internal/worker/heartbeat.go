package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	heartbeatFlushDuration metric.Float64Histogram
	heartbeatFlushErrors   metric.Int64Counter
)

func init() {
	meter := otel.Meter("strait/worker")
	heartbeatFlushDuration, _ = meter.Float64Histogram(
		"strait_worker_heartbeat_flush_duration_seconds",
		metric.WithDescription("Duration of heartbeat batch flush to database"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	heartbeatFlushErrors, _ = meter.Int64Counter(
		"strait_worker_heartbeat_flush_errors_total",
		metric.WithDescription("Heartbeat batch flush failures"),
	)
}

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

func NewHeartbeatSender(s heartbeatLegacyStore, interval time.Duration) *HeartbeatManager {
	if store, ok := s.(HeartbeatStore); ok {
		return NewHeartbeatManager(store, interval)
	}
	return NewHeartbeatManager(heartbeatStoreAdapter{store: s}, interval)
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

func (h *HeartbeatManager) recordOldestLag(ctx context.Context, ids []string) {
	var oldest time.Time
	for _, id := range ids {
		value, ok := h.registeredAt.Load(id)
		if !ok {
			continue
		}
		registeredAt, ok := value.(time.Time)
		if !ok || registeredAt.IsZero() {
			continue
		}
		if oldest.IsZero() || registeredAt.Before(oldest) {
			oldest = registeredAt
		}
	}
	if oldest.IsZero() {
		return
	}
	recordHeartbeatLag(ctx, h.now().Sub(oldest))
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
