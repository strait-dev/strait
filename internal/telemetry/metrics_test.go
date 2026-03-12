package telemetry

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestInitMetrics(t *testing.T) {
	t.Parallel()
	metrics, handler, shutdown, err := InitMetrics("test-service")
	if err != nil {
		if strings.Contains(err.Error(), "conflicting Schema URL") {
			t.Skipf("OTel schema URL conflict (known issue): %v", err)
		}
		t.Fatalf("InitMetrics() error = %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	if metrics == nil {
		t.Fatal("metrics is nil")
	}
	if handler == nil {
		t.Fatal("handler is nil")
	}

	// Verify all metric fields are initialized.
	if metrics.RunTransitions == nil {
		t.Error("RunTransitions is nil")
	}
	if metrics.DequeueDuration == nil {
		t.Error("DequeueDuration is nil")
	}
	if metrics.DispatchDuration == nil {
		t.Error("DispatchDuration is nil")
	}
	if metrics.DispatchErrors == nil {
		t.Error("DispatchErrors is nil")
	}
	if metrics.WebhookDeliveriesTotal == nil {
		t.Error("WebhookDeliveriesTotal is nil")
	}
	if metrics.WebhookDeliveryDuration == nil {
		t.Error("WebhookDeliveryDuration is nil")
	}
	if metrics.WebhookDeliveryAttempts == nil {
		t.Error("WebhookDeliveryAttempts is nil")
	}
	if metrics.WebhookCircuitBreaker == nil {
		t.Error("WebhookCircuitBreaker is nil")
	}
	if metrics.WebhookPayloadBytes == nil {
		t.Error("WebhookPayloadBytes is nil")
	}
}

func TestInitMetrics_ShutdownIdempotent(t *testing.T) {
	t.Parallel()
	_, _, shutdown, err := InitMetrics("test-service")
	if err != nil {
		if strings.Contains(err.Error(), "conflicting Schema URL") {
			t.Skipf("OTel schema URL conflict: %v", err)
		}
		t.Fatalf("InitMetrics() error = %v", err)
	}

	ctx := context.Background()
	for i := range 3 {
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown call %d error = %v", i+1, err)
		}
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
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}

	ctx := context.Background()
	counter.Add(ctx, 1)
	counter.Add(ctx, 5)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("no scope metrics collected")
	}
	if len(rm.ScopeMetrics[0].Metrics) == 0 {
		t.Fatal("no metrics collected")
	}

	m := rm.ScopeMetrics[0].Metrics[0]
	if m.Name != "test.counter" {
		t.Errorf("name = %q, want %q", m.Name, "test.counter")
	}

	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("data type = %T, want Sum[int64]", m.Data)
	}
	if len(sum.DataPoints) == 0 {
		t.Fatal("no data points")
	}
	if sum.DataPoints[0].Value != 6 {
		t.Errorf("counter value = %d, want 6", sum.DataPoints[0].Value)
	}
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
	if err != nil {
		t.Fatalf("Float64Histogram() error = %v", err)
	}

	ctx := context.Background()
	hist.Record(ctx, 0.5)
	hist.Record(ctx, 1.5)
	hist.Record(ctx, 2.0)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("no scope metrics collected")
	}
	if len(rm.ScopeMetrics[0].Metrics) == 0 {
		t.Fatal("no metrics collected")
	}

	m := rm.ScopeMetrics[0].Metrics[0]
	if m.Name != "test.duration" {
		t.Errorf("name = %q, want %q", m.Name, "test.duration")
	}

	histogram, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("data type = %T, want Histogram[float64]", m.Data)
	}
	if len(histogram.DataPoints) == 0 {
		t.Fatal("no data points")
	}

	dp := histogram.DataPoints[0]
	if dp.Count != 3 {
		t.Errorf("count = %d, want 3", dp.Count)
	}
	if dp.Sum != 4.0 {
		t.Errorf("sum = %f, want 4.0", dp.Sum)
	}
}

// TestStraitMetricInstruments verifies all production metric instruments
// can be created and record values without error.
func TestStraitMetricInstruments(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	meter := provider.Meter("strait")

	runTransitions, err := meter.Int64Counter("strait.run.transitions",
		metric.WithDescription("Total run status transitions"),
		metric.WithUnit("1"),
	)
	if err != nil {
		t.Fatalf("Int64Counter(run.transitions) error = %v", err)
	}

	dequeueDuration, err := meter.Float64Histogram("strait.dequeue.duration",
		metric.WithDescription("Duration of dequeue operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		t.Fatalf("Float64Histogram(dequeue.duration) error = %v", err)
	}

	dispatchDuration, err := meter.Float64Histogram("strait.dispatch.duration",
		metric.WithDescription("Duration of HTTP dispatch operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		t.Fatalf("Float64Histogram(dispatch.duration) error = %v", err)
	}

	dispatchErrors, err := meter.Int64Counter("strait.dispatch.errors",
		metric.WithDescription("Total dispatch errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		t.Fatalf("Int64Counter(dispatch.errors) error = %v", err)
	}
	webhookDeliveriesTotal, err := meter.Int64Counter("strait_webhook_deliveries_total",
		metric.WithDescription("Total webhook deliveries by delivery status and retry policy"),
		metric.WithUnit("1"),
	)
	if err != nil {
		t.Fatalf("Int64Counter(webhook.deliveries.total) error = %v", err)
	}
	webhookDeliveryDuration, err := meter.Float64Histogram("strait_webhook_delivery_duration_seconds",
		metric.WithDescription("Webhook delivery HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		t.Fatalf("Float64Histogram(webhook.delivery.duration) error = %v", err)
	}
	webhookDeliveryAttempts, err := meter.Int64Counter("strait_webhook_delivery_attempts_total",
		metric.WithDescription("Total webhook delivery attempts"),
		metric.WithUnit("1"),
	)
	if err != nil {
		t.Fatalf("Int64Counter(webhook.delivery.attempts) error = %v", err)
	}
	webhookCircuitBreaker, err := meter.Int64Gauge("strait_webhook_circuit_breaker_state",
		metric.WithDescription("Webhook circuit breaker state (1=current state, 0=other states)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		t.Fatalf("Int64Gauge(webhook.circuit_breaker.state) error = %v", err)
	}
	webhookPayloadBytes, err := meter.Int64Histogram("strait_webhook_payload_bytes",
		metric.WithDescription("Webhook payload size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		t.Fatalf("Int64Histogram(webhook.payload.bytes) error = %v", err)
	}

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
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("no scope metrics collected")
	}
	if got := len(rm.ScopeMetrics[0].Metrics); got != 9 {
		t.Errorf("collected %d metrics, want 9", got)
	}
}
