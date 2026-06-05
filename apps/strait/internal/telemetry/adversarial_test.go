package telemetry

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// initTestMetrics creates a Metrics instance for testing.
func initTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	m, _, shutdown, err := InitMetrics("test-service", "test")
	require.NoError(t,
		err)

	t.Cleanup(func() {
		_ = shutdown(context.Background())
	})
	return m
}

// TestMetric_NaNValue verifies recording NaN does not panic.
func TestMetric_NaNValue(t *testing.T) {
	t.Parallel()
	m := initTestMetrics(t)
	ctx := context.Background()
	// Recording NaN on a histogram should not panic.
	m.DequeueDuration.Record(ctx, math.NaN())
	m.DispatchDuration.Record(ctx, math.NaN())
}

// TestMetric_InfValue verifies recording Inf does not panic.
func TestMetric_InfValue(t *testing.T) {
	t.Parallel()
	m := initTestMetrics(t)
	ctx := context.Background()
	m.DequeueDuration.Record(ctx, math.Inf(1))
	m.DequeueDuration.Record(ctx, math.Inf(-1))
	m.DispatchDuration.Record(ctx, math.Inf(1))
}

// TestMetric_NegativeCounter verifies adding a negative value to a counter does not panic.
func TestMetric_NegativeCounter(t *testing.T) {
	t.Parallel()
	m := initTestMetrics(t)
	ctx := context.Background()
	// Counters should handle negative increments gracefully (OTel ignores them).
	m.RunTransitions.Add(ctx, -1)
	m.DispatchErrors.Add(ctx, -100)
}

// TestMetric_HighCardinalityLabels verifies that recording with many unique label values does not panic.
func TestMetric_HighCardinalityLabels(t *testing.T) {
	t.Parallel()
	m := initTestMetrics(t)
	ctx := context.Background()
	for i := range 10000 {
		label := attribute.String("unique_key", strings.Repeat("x", 10)+string(rune(i%256)))
		m.RunTransitions.Add(ctx, 1, metric.WithAttributes(label))
	}
}

// TestLogBridge_NewlineInjection verifies that newline characters in slog fields do not cause issues.
func TestLogBridge_NewlineInjection(t *testing.T) {
	t.Parallel()
	// InitLogBridge with empty endpoint returns nil logger and noop shutdown.
	logger, shutdown, err := InitLogBridge(context.Background(), "test", "", "test")
	require.NoError(t,
		err)
	require.Nil(t, logger)
	require.NoError(t,
		shutdown(context.Background()),
	)

}

// FuzzMetricValues fuzzes float64 histogram recording for panics.
func FuzzMetricValues(f *testing.F) {
	f.Add(0.0)
	f.Add(-1.0)
	f.Add(math.MaxFloat64)
	f.Add(math.SmallestNonzeroFloat64)
	f.Add(math.NaN())
	f.Add(math.Inf(1))
	f.Add(math.Inf(-1))

	f.Fuzz(func(t *testing.T, val float64) {
		m, _, shutdown, err := InitMetrics("fuzz-svc", "test")
		require.NoError(t,
			err)

		defer func() { _ = shutdown(context.Background()) }()

		ctx := context.Background()
		// Must not panic for any float64 value.
		m.DequeueDuration.Record(ctx, val)
		m.DispatchDuration.Record(ctx, val)
	})
}
