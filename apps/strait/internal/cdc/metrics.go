package cdc

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	cdcMetricsOnce           sync.Once
	cdcSharedDedupeFallbacks metric.Int64Counter
)

func initCDCMetrics() {
	cdcMetricsOnce.Do(func() {
		meter := otel.Meter("strait/cdc")
		cdcSharedDedupeFallbacks, _ = meter.Int64Counter("strait_cdc_shared_dedupe_fallback_total")
	})
}

func recordSharedDedupeFallback(component string) {
	initCDCMetrics()
	cdcSharedDedupeFallbacks.Add(context.Background(), 1, metric.WithAttributes(attribute.String("component", component)))
}
