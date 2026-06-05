package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAuthRuntimeMetrics_RecordDecisionLabels(t *testing.T) {
	metrics, reader := newAuthMetricsHarness(t)

	metrics.recordDecision(context.Background(), "api_key", "success")
	metrics.recordDecision(context.Background(), "not-a-kind", "not-an-outcome")

	points := collectAuthSumPoints(t, reader, "strait_auth_decisions_total")
	assertMetricPoint(t, points, 1, map[string]string{
		"kind":    "api_key",
		"outcome": "success",
	})
	assertMetricPoint(t, points, 1, map[string]string{
		"kind":    "unknown",
		"outcome": "failure",
	})
}

func TestAuthRuntimeMetrics_RecordTokenAge(t *testing.T) {
	metrics, reader := newAuthMetricsHarness(t)

	metrics.recordTokenAge(context.Background(), "jwt", time.Now().Add(-2*time.Minute))
	metrics.recordTokenAge(context.Background(), "jwt", time.Now().Add(time.Minute))
	metrics.recordTokenAge(context.Background(), "jwt", time.Time{})

	histogram := collectAuthHistogram(t, reader, "strait_auth_token_age_seconds")
	require.Len(t,
		histogram.
			DataPoints,

		1)

	point := histogram.DataPoints[0]
	require.EqualValues(t, 2, point.
		Count)

	assertAttributes(t, point.Attributes.ToSlice(), map[string]string{"kind": "jwt"})
}

func TestAuthRuntimeMetrics_RecordRateLimitThrottled(t *testing.T) {
	metrics, reader := newAuthMetricsHarness(t)

	metrics.recordRateLimitThrottled(context.Background(), "oidc")
	metrics.recordRateLimitThrottled(context.Background(), "unexpected")

	points := collectAuthSumPoints(t, reader, "strait_auth_rate_limit_throttled_total")
	assertMetricPoint(t, points, 1, map[string]string{"scope": "oidc"})
	assertMetricPoint(t, points, 1, map[string]string{"scope": "auth"})
}

func newAuthMetricsHarness(t *testing.T) (authRuntimeMetrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.
			Shutdown(context.Background()))

	})
	return newAuthRuntimeMetrics(provider.Meter("auth-metrics-test")), reader
}

func collectAuthSumPoints(t *testing.T, reader *sdkmetric.ManualReader, name string) []metricdata.DataPoint[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.Background(), &rm))

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				return data.DataPoints
			default:
				require.Failf(t, "test failure", "%s data type = %T, want int64 sum", name, metric.Data)
			}
		}
	}
	require.Failf(t, "test failure",

		"metric %s not collected", name)
	return nil
}

func collectAuthHistogram(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Histogram[float64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.Background(), &rm))

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			require.True(
				t, ok)

			return histogram
		}
	}
	require.Failf(t, "test failure",

		"metric %s not collected", name)
	return metricdata.Histogram[float64]{}
}

func assertMetricPoint(t *testing.T, points []metricdata.DataPoint[int64], value int64, attrs map[string]string) {
	t.Helper()
	for _, point := range points {
		if point.Value != value {
			continue
		}
		if authAttrsMatch(point.Attributes.ToSlice(), attrs) {
			return
		}
	}
	require.Failf(t, "test failure",

		"metric point value=%v attrs=%v not found in %#v", value, attrs, points)
}

func assertAttributes(t *testing.T, got []attribute.KeyValue, want map[string]string) {
	t.Helper()
	require.True(
		t, authAttrsMatch(got,

			want))

}

func authAttrsMatch(got []attribute.KeyValue, want map[string]string) bool {
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
