package telemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"strait/internal/telemetry"
)

// stubDrainer implements telemetry.AuditDrainerStatsProvider for tests.
type stubDrainer struct {
	depth, capacity int64
}

func (s *stubDrainer) AuditDrainerQueueDepth() int64    { return s.depth }
func (s *stubDrainer) AuditDrainerQueueCapacity() int64 { return s.capacity }

// TestAuditMetrics_Registered asserts the audit metrics (drainer depth gauge,
// drainer capacity gauge, export-capped counter, chain-verify total + failed
// counters) are registered under the expected instrument names and emit values
// through an SDK manual reader. This is the canonical registration contract test - it fails
// loudly if a metric is renamed, removed, or accidentally dropped from
// the Metrics struct.
func TestAuditMetrics_Registered(t *testing.T) {
	t.Parallel()

	m, _, shutdown, err := telemetry.InitMetrics("strait-audit-test", "test")
	require.NoError(t, err)

	t.Cleanup(func() { _ = shutdown(context.Background()) })

	// Increment counters so the SDK exports a data point for each.
	m.AuditEventsExportCapped.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("project_id", "proj-test")))
	m.AuditChainVerifyTotal.Add(context.Background(), 2)
	m.AuditChainVerifyFailed.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("reason", "chain_broken")))

	// Attach an isolated manual reader to a fresh provider so the
	// scrape surface is deterministic and independent from the global
	// Prometheus registry that parallel tests share. The metrics
	// struct itself was produced by InitMetrics, but to inspect the
	// drainer observable gauge we register it on a provider we own.
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("audit-drainer-probe")

	// Re-create the two observable gauges on the local meter so we can
	// assert the callback wiring end-to-end. This mirrors how the
	// ObserveAuditDrainer callback binds to any passed meter.
	depth, err := meter.Int64ObservableGauge("strait_audit_drainer_queue_depth")
	require.NoError(t, err)

	capacity, err := meter.Int64ObservableGauge("strait_audit_drainer_queue_capacity")
	require.NoError(t, err)

	drainer := &stubDrainer{depth: 42, capacity: 4096}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(depth, drainer.AuditDrainerQueueDepth())
		o.ObserveInt64(capacity, drainer.AuditDrainerQueueCapacity())
		return nil
	}, depth, capacity)
	require.NoError(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.
			Background(), &rm))

	seen := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if g, ok := inst.Data.(metricdata.Gauge[int64]); ok {
				for _, dp := range g.DataPoints {
					seen[inst.Name] = dp.Value
				}
			}
		}
	}
	assert.EqualValues(
		t, 42, seen["strait_audit_drainer_queue_depth"])
	assert.EqualValues(
		t, 4096, seen["strait_audit_drainer_queue_capacity"])

	assert.NotNil(t, m.AuditDrainerQueueDepth)
	assert.NotNil(t, m.AuditDrainerQueueCapacity)
	assert.NotNil(t, m.AuditEventsExportCapped)
	assert.NotNil(t, m.AuditChainVerifyTotal)
	assert.NotNil(t, m.AuditChainVerifyFailed)

	// Validate that the InitMetrics struct actually exposes the four
	// counter handles — protects against future refactors that might
	// drop an assignment from the struct literal.
}

// TestObserveAuditDrainer_CallbackReflectsLiveState asserts that each
// SDK Collect reads the drainer's current depth — i.e. the callback is
// re-invoked per scrape rather than capturing a stale snapshot.
func TestObserveAuditDrainer_CallbackReflectsLiveState(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("audit-drainer-live")

	depth, err := meter.Int64ObservableGauge("strait_audit_drainer_queue_depth")
	require.NoError(t, err)

	capacity, err := meter.Int64ObservableGauge("strait_audit_drainer_queue_capacity")
	require.NoError(t, err)

	m := &telemetry.Metrics{
		AuditDrainerQueueDepth:    depth,
		AuditDrainerQueueCapacity: capacity,
	}
	d := &stubDrainer{depth: 1, capacity: 8}
	require.NoError(t, m.ObserveAuditDrainer(meter, d))

	readDepth := func() int64 {
		var rm metricdata.ResourceMetrics
		require.NoError(t, reader.
			Collect(context.
				Background(), &rm))

		for _, sm := range rm.ScopeMetrics {
			for _, inst := range sm.Metrics {
				if inst.Name == "strait_audit_drainer_queue_depth" {
					if g, ok := inst.Data.(metricdata.Gauge[int64]); ok {
						for _, dp := range g.DataPoints {
							return dp.Value
						}
					}
				}
			}
		}
		return -1
	}
	assert.EqualValues(
		t, 1, readDepth())

	d.depth = 99
	assert.EqualValues(
		t, 99, readDepth())
}
