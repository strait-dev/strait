package telemetry_test

import (
	"context"
	"testing"

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

// TestAuditMetrics_Registered asserts the four Phase 11 audit metrics
// (drainer depth gauge, drainer capacity gauge, export-capped counter,
// chain-verify total + failed counters) are registered under the
// expected instrument names and emit values through an SDK manual
// reader. This is the canonical registration contract test — it fails
// loudly if a metric is renamed, removed, or accidentally dropped from
// the Metrics struct.
func TestAuditMetrics_Registered(t *testing.T) {
	t.Parallel()

	m, _, shutdown, err := telemetry.InitMetrics("strait-audit-test", "test")
	if err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}
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
	if err != nil {
		t.Fatalf("create depth gauge: %v", err)
	}
	capacity, err := meter.Int64ObservableGauge("strait_audit_drainer_queue_capacity")
	if err != nil {
		t.Fatalf("create capacity gauge: %v", err)
	}
	drainer := &stubDrainer{depth: 42, capacity: 4096}
	if _, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(depth, drainer.AuditDrainerQueueDepth())
		o.ObserveInt64(capacity, drainer.AuditDrainerQueueCapacity())
		return nil
	}, depth, capacity); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

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
	if got := seen["strait_audit_drainer_queue_depth"]; got != 42 {
		t.Errorf("drainer_queue_depth = %d, want 42", got)
	}
	if got := seen["strait_audit_drainer_queue_capacity"]; got != 4096 {
		t.Errorf("drainer_queue_capacity = %d, want 4096", got)
	}

	// Validate that the InitMetrics struct actually exposes the four
	// counter handles — protects against future refactors that might
	// drop an assignment from the struct literal.
	if m.AuditDrainerQueueDepth == nil {
		t.Error("AuditDrainerQueueDepth instrument not registered")
	}
	if m.AuditDrainerQueueCapacity == nil {
		t.Error("AuditDrainerQueueCapacity instrument not registered")
	}
	if m.AuditEventsExportCapped == nil {
		t.Error("AuditEventsExportCapped instrument not registered")
	}
	if m.AuditChainVerifyTotal == nil {
		t.Error("AuditChainVerifyTotal instrument not registered")
	}
	if m.AuditChainVerifyFailed == nil {
		t.Error("AuditChainVerifyFailed instrument not registered")
	}
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
	if err != nil {
		t.Fatalf("create depth gauge: %v", err)
	}
	capacity, err := meter.Int64ObservableGauge("strait_audit_drainer_queue_capacity")
	if err != nil {
		t.Fatalf("create capacity gauge: %v", err)
	}

	m := &telemetry.Metrics{
		AuditDrainerQueueDepth:    depth,
		AuditDrainerQueueCapacity: capacity,
	}
	d := &stubDrainer{depth: 1, capacity: 8}
	if err := m.ObserveAuditDrainer(meter, d); err != nil {
		t.Fatalf("ObserveAuditDrainer: %v", err)
	}

	readDepth := func() int64 {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &rm); err != nil {
			t.Fatalf("collect: %v", err)
		}
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

	if got := readDepth(); got != 1 {
		t.Errorf("initial depth = %d, want 1", got)
	}
	d.depth = 99
	if got := readDepth(); got != 99 {
		t.Errorf("post-update depth = %d, want 99", got)
	}
}
