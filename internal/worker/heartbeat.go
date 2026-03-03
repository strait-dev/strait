package worker

import (
	"context"
	"log/slog"
	"time"

	"orchestrator/internal/store"
)

// HeartbeatSender periodically updates heartbeat_at for a running job.
type HeartbeatSender struct {
	store    store.Store
	interval time.Duration
}

func NewHeartbeatSender(s store.Store, interval time.Duration) *HeartbeatSender {
	return &HeartbeatSender{
		store:    s,
		interval: interval,
	}
}

// Run sends heartbeats for the given run until ctx is cancelled.
// Intended to be run in a goroutine alongside job execution.
func (h *HeartbeatSender) Run(ctx context.Context, runID string) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.store.UpdateHeartbeat(ctx, runID); err != nil {
				slog.Warn("heartbeat failed",
					"run_id", runID,
					"error", err,
				)
			}
		}
	}
}
