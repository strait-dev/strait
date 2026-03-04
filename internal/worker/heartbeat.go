package worker

import (
	"context"
	"log/slog"
	"time"
)

// HeartbeatStore is the subset of store operations needed by HeartbeatSender.
type HeartbeatStore interface {
	UpdateHeartbeat(ctx context.Context, id string) error
}

// HeartbeatSender periodically updates heartbeat_at for a running job.
type HeartbeatSender struct {
	store    HeartbeatStore
	interval time.Duration
}

func NewHeartbeatSender(s HeartbeatStore, interval time.Duration) *HeartbeatSender {
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
