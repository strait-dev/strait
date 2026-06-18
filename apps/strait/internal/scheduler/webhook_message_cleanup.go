package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// WebhookMessageCleanupStore defines store ops for cleaning up old webhook messages.
type WebhookMessageCleanupStore interface {
	DeleteOldWebhookMessages(ctx context.Context, olderThan time.Time) (int64, error)
}

// WebhookMessageCleanup periodically removes old processed webhook message records.
type WebhookMessageCleanup struct {
	store    WebhookMessageCleanupStore
	interval time.Duration
	logger   *slog.Logger
}

// NewWebhookMessageCleanup creates a new cleanup job. Runs every 6 hours by default.
func NewWebhookMessageCleanup(store WebhookMessageCleanupStore, logger *slog.Logger) *WebhookMessageCleanup {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookMessageCleanup{
		store:    store,
		interval: 6 * time.Hour,
		logger:   logger,
	}
}

// Run starts the cleanup loop. Blocks until ctx is canceled.
func (c *WebhookMessageCleanup) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, c.interval, func() {
				c.cleanup(ctx, time.Now())
			})
		}
	}
}

func (c *WebhookMessageCleanup) cleanup(ctx context.Context, now time.Time) {
	cutoff := now.Add(-30 * 24 * time.Hour)
	count, err := c.store.DeleteOldWebhookMessages(ctx, cutoff)
	if err != nil {
		c.logger.Warn("failed to clean up old webhook messages", "error", err)
	} else if count > 0 {
		c.logger.Info("cleaned up old webhook messages", "deleted", count)
	}
}
