package cache

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	cacheMetricsOnce sync.Once
	cacheOpsTotal    metric.Int64Counter
	cacheFailOpen    metric.Int64Counter
	cacheCASRejects  metric.Int64Counter
	cacheBusEvents   metric.Int64Counter
	cacheBusLagMs    metric.Float64Histogram
)

func initCacheMetrics() {
	cacheMetricsOnce.Do(func() {
		meter := otel.Meter("strait/cache")
		cacheOpsTotal, _ = meter.Int64Counter("strait_cache_operations_total")
		cacheFailOpen, _ = meter.Int64Counter("strait_cache_fail_open_total")
		cacheCASRejects, _ = meter.Int64Counter("strait_cache_cas_rejects_total")
		cacheBusEvents, _ = meter.Int64Counter("strait_cachebus_events_total")
		cacheBusLagMs, _ = meter.Float64Histogram("strait_cachebus_lag_ms")
	})
}

func recordCacheOperation(ctx context.Context, namespace, tier, result string) {
	initCacheMetrics()
	if ctx == nil {
		ctx = context.Background()
	}
	cacheOpsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("namespace", namespace),
		attribute.String("tier", tier),
		attribute.String("result", result),
	))
}

func recordCacheFailOpen(ctx context.Context, namespace, operation string) {
	initCacheMetrics()
	if ctx == nil {
		ctx = context.Background()
	}
	cacheFailOpen.Add(ctx, 1, metric.WithAttributes(
		attribute.String("namespace", namespace),
		attribute.String("operation", operation),
	))
}

func recordCacheCASReject(ctx context.Context, namespace string) {
	initCacheMetrics()
	if ctx == nil {
		ctx = context.Background()
	}
	cacheCASRejects.Add(ctx, 1, metric.WithAttributes(attribute.String("namespace", namespace)))
}

func recordCacheBusEvent(ctx context.Context, action, namespace, direction string, sentAt time.Time) {
	initCacheMetrics()
	if ctx == nil {
		ctx = context.Background()
	}
	cacheBusEvents.Add(ctx, 1, metric.WithAttributes(
		attribute.String("action", action),
		attribute.String("namespace", namespace),
		attribute.String("direction", direction),
	))
	if !sentAt.IsZero() && direction == "receive" {
		cacheBusLagMs.Record(ctx, float64(time.Since(sentAt).Milliseconds()), metric.WithAttributes(
			attribute.String("action", action),
			attribute.String("namespace", namespace),
		))
	}
}
