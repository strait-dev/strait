package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
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
		require.NoError(t,
			provider.Shutdown(context.
				Background()))
	})

	hook := newRedisMetricsHook(
		newRedisRuntimeMetrics(provider.Meter("redis-metrics-test")),
		"primary",
		redisMetricsStatsProvider{stats: &redis.PoolStats{TotalConns: 5, IdleConns: 2}},
	)
	ctx := context.Background()
	require.NoError(t,
		hook.ProcessHook(func(
			context.Context, redis.Cmder) error {
			return nil
		})(ctx, redis.NewStringCmd(ctx, "GET", "cache-key")))

	if err := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return redis.Nil
	})(ctx, redis.NewStringCmd(ctx, "GET", "missing-key")); !errors.Is(err, redis.Nil) {
		require.ErrorIs(t, err, redis.Nil)
	}
	require.Error(t, hook.
		ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
			return errors.New("redis unavailable")
		})(ctx, []redis.Cmder{redis.NewStringCmd(ctx, "SET", "cache-key", "value")}))

	histogram := collectRedisHistogram(t, reader, "strait_redis_command_duration_seconds")
	assertRedisHistogramPoint(t, histogram, map[string]string{"command": "get", "outcome": "success"})
	assertRedisHistogramPoint(t, histogram, map[string]string{"command": "get", "outcome": "miss"})
	assertRedisHistogramPoint(t, histogram, map[string]string{"command": "pipeline", "outcome": "error"})

	gauge := collectRedisGauge(t, reader, "strait_redis_pool_active")
	assertRedisGaugePoint(t, gauge, 3, map[string]string{"pool": "primary"})
}

func TestRedisMetricsNormalization(t *testing.T) {
	require.Equal(t, "client",
		normalizeRedisCommand(" CLIENT LIST "))
	require.Equal(t, "unknown",
		normalizeRedisCommand(""))
	require.Equal(t, "miss",
		redisCommandOutcome(redis.Nil))
	require.Equal(t, "error",
		redisCommandOutcome(errors.New("boom")))
}

func collectRedisHistogram(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Histogram[float64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			require.True(t, ok)

			return histogram
		}
	}
	require.Failf(t, "metric not collected", "%s", name)
	return metricdata.Histogram[float64]{}
}

func collectRedisGauge(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Gauge[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			gauge, ok := metric.Data.(metricdata.Gauge[int64])
			require.True(t, ok)

			return gauge
		}
	}
	require.Failf(t, "metric not collected", "%s", name)
	return metricdata.Gauge[int64]{}
}

func assertRedisHistogramPoint(t *testing.T, histogram metricdata.Histogram[float64], attrs map[string]string) {
	t.Helper()
	for _, point := range histogram.DataPoints {
		if redisAttrsMatch(point.Attributes.ToSlice(), attrs) && point.Count > 0 {
			return
		}
	}
	require.Failf(t, "histogram point not found", "attrs=%v points=%#v", attrs, histogram.DataPoints)
}

func assertRedisGaugePoint(t *testing.T, gauge metricdata.Gauge[int64], value int64, attrs map[string]string) {
	t.Helper()
	for _, point := range gauge.DataPoints {
		if point.Value == value && redisAttrsMatch(point.Attributes.ToSlice(), attrs) {
			return
		}
	}
	require.Failf(t, "gauge point not found", "value=%d attrs=%v points=%#v", value, attrs, gauge.DataPoints)
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
