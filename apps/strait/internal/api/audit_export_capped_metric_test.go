package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/telemetry"
)

// auditMetricsHarness wires a *telemetry.Metrics backed by a manual
// SDK reader so tests can assert counter increments deterministically
// without depending on the global Prometheus registry.
type auditMetricsHarness struct {
	metrics *telemetry.Metrics
	reader  *sdkmetric.ManualReader
}

func newAuditMetricsHarness(t *testing.T) *auditMetricsHarness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("audit-metrics-harness")

	exportCapped, err := meter.Int64Counter("strait_audit_events_export_capped_total")
	require.NoError(t, err)

	verifyTotal, err := meter.Int64Counter("strait_audit_chain_verify_total")
	require.NoError(t, err)

	verifyFailed, err := meter.Int64Counter("strait_audit_chain_verify_failed_total")
	require.NoError(t, err)

	// The HTTP-layer middleware blindly dereferences its instruments
	// whenever metrics is non-nil, so tests that route a request
	// through chi must populate those too or accept a nil panic.
	httpDuration, err := meter.Float64Histogram("strait_http_request_duration_seconds")
	require.NoError(t, err)

	httpInflight, err := meter.Int64UpDownCounter("strait_http_inflight_requests")
	require.NoError(t, err)

	return &auditMetricsHarness{
		metrics: &telemetry.Metrics{
			AuditEventsExportCapped: exportCapped,
			AuditChainVerifyTotal:   verifyTotal,
			AuditChainVerifyFailed:  verifyFailed,
			HTTPRequestDuration:     httpDuration,
			HTTPInflightRequests:    httpInflight,
		},
		reader: reader,
	}
}

// sumCounter totals all data points for the given instrument name.
func (h *auditMetricsHarness) sumCounter(t *testing.T, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, h.reader.
		Collect(context.Background(),
			&rm))

	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if inst.Name != name {
				continue
			}
			if sum, ok := inst.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range sum.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

// TestAuditExport_CapHit_IncrementsExportCappedCounter drives the full
// export handler with a 2-row cap, 5 events upstream, and asserts
// strait_audit_events_export_capped_total fires exactly once (one
// export that tripped the cap). The counter feeds the Grafana 24h increase
// panel.
func TestAuditExport_CapHit_IncrementsExportCappedCounter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			for i := range 5 {
				ev := &domain.AuditEvent{
					ID:        "ev-" + itoaBench(i),
					ProjectID: "proj-cap",
					Action:    "job.created",
					CreatedAt: now,
				}
				if err := fn(ev); err != nil {
					return err
				}
			}
			return nil
		},
	}

	h := newAuditMetricsHarness(t)
	// Set AuditExportRowCapDefault to 2 so the test can realistically
	// trip the cap without streaming 1M synthetic events.
	cfg := &config.Config{
		InternalSecret:           "test-secret-value",
		MaxBulkTriggerItems:      500,
		JWTSigningKey:            testJWTSigningKey,
		AuditExportRowCapDefault: 2,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Metrics: h.metrics,
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson",
		"", "proj-cap")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	got := h.sumCounter(t, "strait_audit_events_export_capped_total")
	assert.EqualValues(t, 1, got)
}

// TestAuditExport_NoCap_DoesNotIncrementExportCappedCounter verifies
// the counter stays at zero when the export completes under the cap.
// A counter that increments on every export regardless of outcome
// would poison the 24h increase dashboard and silently conflate the
// happy path with the saturation path.
func TestAuditExport_NoCap_DoesNotIncrementExportCappedCounter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{
				ID: "ev-1", ProjectID: "proj-nocap", Action: "job.created", CreatedAt: now,
			})
		},
	}

	h := newAuditMetricsHarness(t)
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{Config: cfg, Store: ms, Metrics: h.metrics})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet,
		"/v1/audit-events/export?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z&format=ndjson",
		"", "proj-nocap")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	assert.EqualValues(t, 0, h.sumCounter(t, "strait_audit_events_export_capped_total"))
}
