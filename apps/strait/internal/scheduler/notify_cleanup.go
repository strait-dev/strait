package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type notifyCleanupStore interface {
	DeleteExpiredNotifyProviderCallbackReceipts(ctx context.Context, limit int) (int64, error)
	DeleteOldNotifySuppressionEvents(ctx context.Context, before time.Time, limit int) (int64, error)
}

type NotifyCleanup struct {
	store                notifyCleanupStore
	interval             time.Duration
	suppressionRetention time.Duration
	batchSize            int
	logger               *slog.Logger
	metrics              *telemetry.Metrics
}

func NewNotifyCleanup(store notifyCleanupStore, interval, suppressionRetention time.Duration, batchSize int) *NotifyCleanup {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	if suppressionRetention <= 0 {
		suppressionRetention = 30 * 24 * time.Hour
	}
	if batchSize <= 0 {
		batchSize = 1000
	}

	return &NotifyCleanup{
		store:                store,
		interval:             interval,
		suppressionRetention: suppressionRetention,
		batchSize:            batchSize,
		logger:               slog.Default(),
	}
}

func (c *NotifyCleanup) WithMetrics(m *telemetry.Metrics) *NotifyCleanup {
	c.metrics = m
	return c
}

func (c *NotifyCleanup) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *NotifyCleanup) cleanup(ctx context.Context) {
	c.cleanupExpiredReceipts(ctx)
	c.cleanupOldSuppressionEvents(ctx)
}

func (c *NotifyCleanup) cleanupExpiredReceipts(ctx context.Context) {
	var total int64
	for {
		deleted, err := c.store.DeleteExpiredNotifyProviderCallbackReceipts(ctx, c.batchSize)
		if err != nil {
			c.logger.Warn("notify cleanup: delete expired callback receipts failed", "error", err)
			return
		}
		total += deleted
		if deleted < int64(c.batchSize) {
			break
		}
	}
	if total > 0 {
		c.logger.Info("notify cleanup: deleted expired callback receipts", "deleted", total)
		c.recordCleanupMetric(ctx, "callback_receipts", total)
	}
}

func (c *NotifyCleanup) cleanupOldSuppressionEvents(ctx context.Context) {
	before := time.Now().UTC().Add(-c.suppressionRetention)
	var total int64
	for {
		deleted, err := c.store.DeleteOldNotifySuppressionEvents(ctx, before, c.batchSize)
		if err != nil {
			c.logger.Warn("notify cleanup: delete old suppression events failed", "error", err)
			return
		}
		total += deleted
		if deleted < int64(c.batchSize) {
			break
		}
	}
	if total > 0 {
		c.logger.Info("notify cleanup: deleted old suppression events", "deleted", total, "retention", c.suppressionRetention.String())
		c.recordCleanupMetric(ctx, "suppression_events", total)
	}
}

func (c *NotifyCleanup) recordCleanupMetric(ctx context.Context, table string, deleted int64) {
	if c == nil || c.metrics == nil || c.metrics.NotifyCleanupDeletedTotal == nil || deleted <= 0 {
		return
	}
	c.metrics.NotifyCleanupDeletedTotal.Add(ctx, deleted, metric.WithAttributes(attribute.String("table", table)))
}
