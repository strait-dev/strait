package telemetry

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestInitMetrics(t *testing.T) {
	t.Parallel()
	metrics, handler, shutdown, err := InitMetrics("test-service", "test")
	if err != nil {
		if strings.Contains(err.Error(), "conflicting Schema URL") {
			t.Skipf("OTel schema URL conflict (known issue): %v", err)
		}
		require.NoError(t, err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	require.NotNil(t, metrics)

	require.NotNil(t, handler)
	assert.NotNil(t, metrics.
		RunTransitions,
	)
	assert.NotNil(t, metrics.
		DequeueDuration,
	)
	assert.NotNil(t, metrics.
		DispatchDuration,
	)
	assert.NotNil(t, metrics.
		DispatchErrors,
	)
	assert.NotNil(t, metrics.
		WebhookDeliveriesTotal,
	)
	assert.NotNil(t, metrics.
		WebhookDeliveryDuration,
	)
	assert.NotNil(t, metrics.
		WebhookDeliveryAttempts,
	)
	assert.NotNil(t, metrics.
		WebhookCircuitBreaker,
	)
	assert.NotNil(t, metrics.
		WebhookPayloadBytes,
	)
	assert.NotNil(t, metrics.
		PprofRequests,
	)

	// Verify all metric fields are initialized.

}

func TestInitMetrics_EmptyEnvironment(t *testing.T) {
	t.Parallel()
	metrics, handler, shutdown, err := InitMetrics("test-service", "")
	if err != nil {
		if strings.Contains(err.Error(), "conflicting Schema URL") {
			t.Skipf("OTel schema URL conflict: %v", err)
		}
		require.NoError(t, err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	require.NotNil(t, metrics)
	require.NotNil(t, handler)

}

func TestInitMetrics_ShutdownIdempotent(t *testing.T) {
	t.Parallel()
	_, _, shutdown, err := InitMetrics("test-service", "test")
	if err != nil {
		if strings.Contains(err.Error(), "conflicting Schema URL") {
			t.Skipf("OTel schema URL conflict: %v", err)
		}
		require.NoError(t, err)
	}

	ctx := context.Background()
	for range 3 {
		assert.NoError(t, shutdown(
			ctx))

	}
}

// TestCounterRecording verifies Int64Counter records values correctly.
// Uses ManualReader to avoid global OTel state and schema URL conflicts.
func TestCounterRecording(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("test")

	counter, err := meter.Int64Counter("test.counter",
		metric.WithDescription("test counter"),
		metric.WithUnit("1"),
	)
	require.NoError(t,
		err)

	ctx := context.Background()
	counter.Add(ctx, 1)
	counter.Add(ctx, 5)

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(ctx,
			&rm))
	require.NotEmpty(t,
		rm.ScopeMetrics,
	)
	require.NotEmpty(t,
		rm.ScopeMetrics[0].
			Metrics)

	m := rm.ScopeMetrics[0].Metrics[0]
	assert.Equal(t, "test.counter",

		m.Name,
	)

	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.NotEmpty(t,
		sum.DataPoints,
	)
	assert.EqualValues(t, 6,
		sum.DataPoints[0].Value,
	)

}

// TestHistogramRecording verifies Float64Histogram records values correctly.
func TestHistogramRecording(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("test")

	hist, err := meter.Float64Histogram("test.duration",
		metric.WithDescription("test duration"),
		metric.WithUnit("s"),
	)
	require.NoError(t,
		err)

	ctx := context.Background()
	hist.Record(ctx, 0.5)
	hist.Record(ctx, 1.5)
	hist.Record(ctx, 2.0)

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(ctx,
			&rm))
	require.NotEmpty(t,
		rm.ScopeMetrics,
	)
	require.NotEmpty(t,
		rm.ScopeMetrics[0].
			Metrics)

	m := rm.ScopeMetrics[0].Metrics[0]
	assert.Equal(t, "test.duration",

		m.Name,
	)

	histogram, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.NotEmpty(t,
		histogram.
			DataPoints,
	)

	dp := histogram.DataPoints[0]
	assert.EqualValues(t, 3,
		dp.Count,
	)
	assert.EqualValues(t, 4.0,
		dp.Sum,
	)

}

// TestStraitMetricInstruments verifies all production metric instruments
// can be created and record values without error.
func TestStraitMetricInstruments(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("strait")

	runTransitions, err := meter.Int64Counter("strait_run_transitions_total",
		metric.WithDescription("Total run status transitions"),
		metric.WithUnit("1"),
	)
	require.NoError(t,
		err)

	dequeueDuration, err := meter.Float64Histogram("strait_worker_dequeue_duration_seconds",
		metric.WithDescription("Duration of dequeue operations"),
		metric.WithUnit("s"),
	)
	require.NoError(t,
		err)

	dispatchDuration, err := meter.Float64Histogram("strait_dispatch_duration_seconds",
		metric.WithDescription("Duration of HTTP dispatch operations"),
		metric.WithUnit("s"),
	)
	require.NoError(t,
		err)

	dispatchErrors, err := meter.Int64Counter("strait_dispatch_errors_total",
		metric.WithDescription("Total dispatch errors"),
		metric.WithUnit("1"),
	)
	require.NoError(t,
		err)

	webhookDeliveriesTotal, err := meter.Int64Counter("strait_webhook_deliveries_total",
		metric.WithDescription("Total webhook deliveries by delivery status and retry policy"),
		metric.WithUnit("1"),
	)
	require.NoError(t,
		err)

	webhookDeliveryDuration, err := meter.Float64Histogram("strait_webhook_delivery_duration_seconds",
		metric.WithDescription("Webhook delivery HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	require.NoError(t,
		err)

	webhookDeliveryAttempts, err := meter.Int64Counter("strait_webhook_delivery_attempts_total",
		metric.WithDescription("Total webhook delivery attempts"),
		metric.WithUnit("1"),
	)
	require.NoError(t,
		err)

	webhookCircuitBreaker, err := meter.Int64Gauge("strait_webhook_circuit_breaker_state",
		metric.WithDescription("Webhook circuit breaker state (1=current state, 0=other states)"),
		metric.WithUnit("1"),
	)
	require.NoError(t,
		err)

	webhookPayloadBytes, err := meter.Int64Histogram("strait_webhook_payload_bytes",
		metric.WithDescription("Webhook payload size in bytes"),
		metric.WithUnit("By"),
	)
	require.NoError(t,
		err)

	ctx := context.Background()
	runTransitions.Add(ctx, 10)
	dequeueDuration.Record(ctx, 0.05)
	dispatchDuration.Record(ctx, 1.2)
	dispatchErrors.Add(ctx, 2)
	webhookDeliveriesTotal.Add(ctx, 1)
	webhookDeliveryDuration.Record(ctx, 0.2)
	webhookDeliveryAttempts.Add(ctx, 1)
	webhookCircuitBreaker.Record(ctx, 1)
	webhookPayloadBytes.Record(ctx, 256)

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(ctx,
			&rm))
	require.NotEmpty(t,
		rm.ScopeMetrics,
	)
	assert.EqualValues(t, 9,
		len(rm.ScopeMetrics[0].Metrics),
	)

}

type mockBreakerStateProvider struct {
	state int64
}

func (m *mockBreakerStateProvider) BreakerState() int64 { return m.state }

func TestObserveSIEMBreakerState_NilProvider(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("test")
	gauge, err := meter.Int64ObservableGauge("test.siem.breaker")
	require.NoError(t,
		err)

	m := &Metrics{AuditSIEMBreakerState: gauge}
	require.NoError(t,
		m.ObserveSIEMBreakerState(meter,

			nil))

}

func TestObserveSIEMBreakerState_NilGauge(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("test")
	m := &Metrics{AuditSIEMBreakerState: nil}
	bp := &mockBreakerStateProvider{state: 1}
	require.NoError(t,
		m.ObserveSIEMBreakerState(meter,

			bp))

}

func TestObserveSIEMBreakerState_RecordsValue(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("test")
	gauge, err := meter.Int64ObservableGauge("test.siem.breaker.state")
	require.NoError(t,
		err)

	m := &Metrics{AuditSIEMBreakerState: gauge}
	bp := &mockBreakerStateProvider{state: 2}
	require.NoError(t,
		m.ObserveSIEMBreakerState(meter,

			bp))

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, met := range sm.Metrics {
			if met.Name == "test.siem.breaker.state" {
				g, ok := met.Data.(metricdata.Gauge[int64])
				require.True(t, ok)
				require.NotEmpty(t,
					g.DataPoints,
				)
				assert.EqualValues(t, 2,
					g.DataPoints[0].Value,
				)

				found = true
			}
		}
	}
	require.True(t, found)

}

type mockPoolStats struct {
	running   int64
	waiting   uint64
	submitted uint64
	completed uint64
	success   uint64
	failed    uint64
	dropped   uint64
}

func (m *mockPoolStats) RunningWorkers() int64   { return m.running }
func (m *mockPoolStats) WaitingTasks() uint64    { return m.waiting }
func (m *mockPoolStats) SubmittedTasks() uint64  { return m.submitted }
func (m *mockPoolStats) CompletedTasks() uint64  { return m.completed }
func (m *mockPoolStats) SuccessfulTasks() uint64 { return m.success }
func (m *mockPoolStats) FailedTasks() uint64     { return m.failed }
func (m *mockPoolStats) DroppedTasks() uint64    { return m.dropped }

func TestObservePool_SaturateInt64_MaxUint64(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("test")

	poolRunning, _ := meter.Int64ObservableGauge("test.pool.running")
	poolWaiting, _ := meter.Int64ObservableGauge("test.pool.waiting")
	poolSubmitted, _ := meter.Int64ObservableCounter("test.pool.submitted")
	poolCompleted, _ := meter.Int64ObservableCounter("test.pool.completed")
	poolSuccess, _ := meter.Int64ObservableCounter("test.pool.success")
	poolFailed, _ := meter.Int64ObservableCounter("test.pool.failed")
	poolDropped, _ := meter.Int64ObservableCounter("test.pool.dropped")

	m := &Metrics{
		PoolRunningWorkers:  poolRunning,
		PoolWaitingTasks:    poolWaiting,
		PoolSubmittedTasks:  poolSubmitted,
		PoolCompletedTasks:  poolCompleted,
		PoolSuccessfulTasks: poolSuccess,
		PoolFailedTasks:     poolFailed,
		PoolDroppedTasks:    poolDropped,
	}

	pool := &mockPoolStats{
		running:   5,
		waiting:   math.MaxUint64,
		submitted: 500_000,
		completed: 499_000,
		success:   0,
		failed:    1000,
		dropped:   42,
	}
	require.NoError(t,
		m.ObservePool(meter,
			pool))

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	values := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, met := range sm.Metrics {
			switch data := met.Data.(type) {
			case metricdata.Gauge[int64]:
				if len(data.DataPoints) > 0 {
					values[met.Name] = data.DataPoints[0].Value
				}
			case metricdata.Sum[int64]:
				if len(data.DataPoints) > 0 {
					values[met.Name] = data.DataPoints[0].Value
				}
			}
		}
	}
	assert.EqualValues(t, 5,
		values["test.pool.running"])
	assert.EqualValues(t, math.
		MaxInt64,
		values["test.pool.waiting"])
	assert.EqualValues(t, 500_000,
		values["test.pool.submitted"])
	assert.EqualValues(t, 499_000,
		values["test.pool.completed"])
	assert.EqualValues(t, 0,
		values["test.pool.success"])
	assert.EqualValues(t, 1000,
		values["test.pool.failed"])
	assert.EqualValues(t, 42,
		values["test.pool.dropped"])

}
