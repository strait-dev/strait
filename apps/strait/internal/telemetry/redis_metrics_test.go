package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type redisMetricsStatsProvider struct {
	stats *redis.PoolStats
}

func (p redisMetricsStatsProvider) PoolStats() *redis.PoolStats {
	return p.stats
}

func TestRedisMetricsHookRecordsCommandOutcomesAndPool(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown meter provider: %v", err)
		}
	})

	hook := newRedisMetricsHook(
		newRedisRuntimeMetrics(provider.Meter("redis-metrics-test")),
		"primary",
		redisMetricsStatsProvider{stats: &redis.PoolStats{TotalConns: 5, IdleConns: 2}},
	)
	ctx := context.Background()

	if err := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return nil
	})(ctx, redis.NewStringCmd(ctx, "GET", "cache-key")); err != nil {
		t.Fatalf("ProcessHook success returned error: %v", err)
	}
	if err := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return redis.Nil
	})(ctx, redis.NewStringCmd(ctx, "GET", "missing-key")); !errors.Is(err, redis.Nil) {
		t.Fatalf("ProcessHook miss error = %v, want redis.Nil", err)
	}
	if err := hook.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return errors.New("redis unavailable")
	})(ctx, []redis.Cmder{redis.NewStringCmd(ctx, "SET", "cache-key", "value")}); err == nil {
		t.Fatal("ProcessPipelineHook error = nil, want error")
	}

	histogram := collectRedisHistogram(t, reader, "strait_redis_command_duration_seconds")
	assertRedisHistogramPoint(t, histogram, map[string]string{"command": "get", "outcome": "success"})
	assertRedisHistogramPoint(t, histogram, map[string]string{"command": "get", "outcome": "miss"})
	assertRedisHistogramPoint(t, histogram, map[string]string{"command": "pipeline", "outcome": "error"})

	gauge := collectRedisGauge(t, reader, "strait_redis_pool_active")
	assertRedisGaugePoint(t, gauge, 3, map[string]string{"pool": "primary"})
}

func TestRedisMetricsNormalization(t *testing.T) {
	if got := normalizeRedisCommand(" CLIENT LIST "); got != "client" {
		t.Fatalf("normalizeRedisCommand() = %q, want client", got)
	}
	if got := normalizeRedisCommand(""); got != "unknown" {
		t.Fatalf("normalizeRedisCommand(empty) = %q, want unknown", got)
	}
	if got := redisCommandOutcome(redis.Nil); got != "miss" {
		t.Fatalf("redisCommandOutcome(redis.Nil) = %q, want miss", got)
	}
	if got := redisCommandOutcome(errors.New("boom")); got != "error" {
		t.Fatalf("redisCommandOutcome(error) = %q, want error", got)
	}
}

func collectRedisHistogram(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Histogram[float64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("%s data type = %T, want histogram", name, metric.Data)
			}
			return histogram
		}
	}
	t.Fatalf("metric %s not collected", name)
	return metricdata.Histogram[float64]{}
}

func collectRedisGauge(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Gauge[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			gauge, ok := metric.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("%s data type = %T, want int64 gauge", name, metric.Data)
			}
			return gauge
		}
	}
	t.Fatalf("metric %s not collected", name)
	return metricdata.Gauge[int64]{}
}

func assertRedisHistogramPoint(t *testing.T, histogram metricdata.Histogram[float64], attrs map[string]string) {
	t.Helper()
	for _, point := range histogram.DataPoints {
		if redisAttrsMatch(point.Attributes.ToSlice(), attrs) && point.Count > 0 {
			return
		}
	}
	t.Fatalf("histogram point attrs=%v not found in %#v", attrs, histogram.DataPoints)
}

func assertRedisGaugePoint(t *testing.T, gauge metricdata.Gauge[int64], value int64, attrs map[string]string) {
	t.Helper()
	for _, point := range gauge.DataPoints {
		if point.Value == value && redisAttrsMatch(point.Attributes.ToSlice(), attrs) {
			return
		}
	}
	t.Fatalf("gauge point value=%d attrs=%v not found in %#v", value, attrs, gauge.DataPoints)
}

func redisAttrsMatch(got []attribute.KeyValue, want map[string]string) bool {
	values := make(map[string]string, len(got))
	for _, kv := range got {
		values[string(kv.Key)] = kv.Value.AsString()
	}
	for key, wantValue := range want {
		if values[key] != wantValue {
			return false
		}
	}
	return true
}
