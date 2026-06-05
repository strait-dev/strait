package api

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// backpressureMetricsHarness wires the AuditEventsDropped and
// AuditEventsSyncFallback counters against a manual SDK reader so the
// backpressure metric split can be validated deterministically.
type backpressureMetricsHarness struct {
	metrics *telemetry.Metrics
	reader  *sdkmetric.ManualReader
}

func newBackpressureMetricsHarness(t *testing.T) *backpressureMetricsHarness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("backpressure-metrics-harness")

	dropped, err := meter.Int64Counter("strait_audit_events_dropped_total")
	require.NoError(t, err)

	syncFallback, err := meter.Int64Counter("strait_audit_events_sync_fallback_total")
	require.NoError(t, err)

	return &backpressureMetricsHarness{
		metrics: &telemetry.Metrics{
			AuditEventsDropped:      dropped,
			AuditEventsSyncFallback: syncFallback,
		},
		reader: reader,
	}
}

// sumCounterByAttr totals the data points for an instrument that match
// attribute key=value. Returns 0 if the instrument has no points.
func (h *backpressureMetricsHarness) sumCounterByAttr(t *testing.T, name, attrKey, attrVal string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, h.reader.
		Collect(context.
			Background(),
			&rm))

	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if inst.Name != name {
				continue
			}
			sum, ok := inst.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				v, present := dp.Attributes.Value(attribute.Key(attrKey))
				if !present {
					continue
				}
				if v.AsString() == attrVal {
					total += dp.Value
				}
			}
		}
	}
	return total
}

// withShortRetries replaces auditRetryDelays with near-zero sleeps for the
// duration of a test. Restored on cleanup. Tests run fast and deterministic.
func withShortRetries(t *testing.T) {
	t.Helper()
	orig := auditRetryDelays
	auditRetryDelays = []time.Duration{
		1 * time.Millisecond,
		1 * time.Millisecond,
		1 * time.Millisecond,
	}
	t.Cleanup(func() {
		auditRetryDelays = orig
	})
}

// TestDrainer_RetriesTransientErrors: store fails twice then succeeds.
// Expectation: event is written exactly once, no deadletter writes.
func TestDrainer_RetriesTransientErrors(t *testing.T) {
	withShortRetries(t)

	var attempts, writes, dlqCalls atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			n := attempts.Add(1)
			if n <= 2 {
				return errors.New("transient")
			}
			writes.Add(1)
			return nil
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, _ *domain.AuditEvent, _ string, _ int) error {
			dlqCalls.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", map[string]any{"run_id": "r1"})
	srv.Close()
	assert.EqualValues(t, 1, writes.
		Load())
	assert.EqualValues(t, 0, dlqCalls.
		Load(),
	)
}

// TestDrainer_DeadlettersAfterExhaustingRetries: store always fails.
// Expectation: event ends up in the deadletter with retry_count=3.
func TestDrainer_DeadlettersAfterExhaustingRetries(t *testing.T) {
	withShortRetries(t)

	var mu sync.Mutex
	var captured struct {
		ev         *domain.AuditEvent
		lastErr    string
		retryCount int
	}
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("db down")
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, ev *domain.AuditEvent, lastErr string, retryCount int) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured.ev = &clone
			captured.lastErr = lastErr
			captured.retryCount = retryCount
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", map[string]any{"run_id": "r1"})
	srv.Close()

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured.
		ev)
	assert.Equal(
		t, domain.
			AuditActionJobTriggered,

		captured.
			ev.Action)
	assert.Equal(
		t, "db down",
		captured.
			lastErr,
	)
	assert.Equal(
		t, len(auditRetryDelays), captured.
			retryCount,
	)
}

// TestDrainer_LogsIfDeadletterAlsoFails: both primary and DLQ fail.
// Expectation: no crash, event is lost with deadletter_failed counter.
func TestDrainer_LogsIfDeadletterAlsoFails(t *testing.T) {
	withShortRetries(t)

	var dlqCalls atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("db down")
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, _ *domain.AuditEvent, _ string, _ int) error {
			dlqCalls.Add(1)
			return errors.New("dlq also down")
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", nil)
	srv.Close()
	assert.EqualValues(t, 1, dlqCalls.
		Load(),
	)
}

// TestDrainer_RetriesDoNotReorderEvents: submit 5 events, first one
// requires 2 retries. All 5 must be written in submission order.
// The retry blocks the drainer (documented trade-off).
func TestDrainer_RetriesDoNotReorderEvents(t *testing.T) {
	withShortRetries(t)

	var mu sync.Mutex
	var writeOrder []string
	var firstAttempt atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.ResourceID == "evt-1" {
				n := firstAttempt.Add(1)
				if n <= 2 {
					return errors.New("transient")
				}
			}
			mu.Lock()
			defer mu.Unlock()
			writeOrder = append(writeOrder, ev.ResourceID)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	ids := []string{"evt-1", "evt-2", "evt-3", "evt-4", "evt-5"}
	for _, id := range ids {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", id, nil)
	}
	srv.Close()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		writeOrder,
		5)

	for i, id := range ids {
		assert.Equal(
			t, id, writeOrder[i])
	}
}

// TestBackpressure_MetricSplit_SuccessOutcome forces the sync-fallback
// path by holding the drainer in a slow store call so the buffer fills
// past the 75% gate, then asserts:
//   - AuditEventsDropped{reason="backpressure_degraded"} fires
//   - legacy "backpressure_sync_fallback" reason is gone
//   - AuditEventsSyncFallback{outcome="success"} fires
//   - AuditEventsSyncFallback{outcome="failure"} stays at 0
//
// The reason rename pairs with the new outcome counter so success and
// failure paths are independently observable. Without the split, the
// legacy reason fired even when the sync write succeeded — a false
// positive on every backpressure trigger.
func TestBackpressure_MetricSplit_SuccessOutcome(t *testing.T) {
	withShortRetries(t)

	// blockDrainer holds the drainer in CreateAuditEvent until released
	// so the buffered channel fills above the 75% threshold and emits
	// take the sync-fallback branch. The sync fallback path uses a
	// SEPARATE store call (ev arg flows through), and that path runs
	// outside the gating select{}, so it can complete even while the
	// drainer is blocked. To keep the success outcome deterministic, we
	// only block the drainer's first call: subsequent calls (sync
	// fallback writes from the request goroutines) succeed immediately.
	var firstCall atomic.Bool
	release := make(chan struct{})
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			if firstCall.CompareAndSwap(false, true) {
				<-release
			}
			return nil
		},
	}
	h := newBackpressureMetricsHarness(t)
	srv := NewServer(ServerDeps{
		Config:               &config.Config{InternalSecret: "test", JWTSigningKey: testJWTSigningKey},
		Store:                ms,
		Metrics:              h.metrics,
		AuditAsyncBufferSize: 256, // minimum allowed
	})
	t.Cleanup(func() {
		// Release the drainer so Close can finish promptly.
		select {
		case <-release:
		default:
			close(release)
		}
		srv.Close()
	})

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	// Send one event and wait for the drainer to pick it up (block on
	// <-release). This ensures the firstCall gate fires inside the drainer,
	// NOT in a sync-fallback call from the loop below. Without this yield
	// the drainer goroutine may not be scheduled before the buffer exceeds
	// 75%, causing the firstCall CAS to fire on the sync path and
	// deadlocking the test body.
	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	for !firstCall.Load() {
		runtime.Gosched()
	}

	// Fill past 256*0.75 = 192 to trigger the backpressure gate.
	// Drainer is blocked on the first event, so the buffer accumulates.
	for range 249 {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	}
	assert.NotEqual(t, 0, h.
		sumCounterByAttr(t,
			"strait_audit_events_dropped_total",

			"reason", "backpressure_degraded",
		))
	assert.EqualValues(t, 0, h.sumCounterByAttr(t, "strait_audit_events_dropped_total",

		"reason", "backpressure_sync_fallback",
	))
	assert.NotEqual(t, 0, h.
		sumCounterByAttr(t,
			"strait_audit_events_sync_fallback_total",

			"outcome", "success",
		))
	assert.EqualValues(t, 0, h.sumCounterByAttr(t, "strait_audit_events_sync_fallback_total",

		"outcome", "failure",
	))
}

// TestBackpressure_MetricSplit_FailureOutcome forces the sync-fallback
// path with a failing store and asserts the failure outcome is recorded
// distinctly from the success outcome.
func TestBackpressure_MetricSplit_FailureOutcome(t *testing.T) {
	withShortRetries(t)

	var firstCall atomic.Bool
	release := make(chan struct{})
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			if firstCall.CompareAndSwap(false, true) {
				<-release
				return nil
			}
			return errors.New("store down")
		},
	}
	h := newBackpressureMetricsHarness(t)
	srv := NewServer(ServerDeps{
		Config:               &config.Config{InternalSecret: "test", JWTSigningKey: testJWTSigningKey},
		Store:                ms,
		Metrics:              h.metrics,
		AuditAsyncBufferSize: 256,
	})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		srv.Close()
	})

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	// Ensure the drainer consumes the first event before filling the
	// buffer, same reason as TestBackpressure_MetricSplit_SuccessOutcome.
	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	for !firstCall.Load() {
		runtime.Gosched()
	}

	for range 249 {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	}
	assert.NotEqual(t, 0, h.
		sumCounterByAttr(t,
			"strait_audit_events_sync_fallback_total",

			"outcome", "failure",
		))
}

// TestDrainer_RetryMetricIncremented: store fails twice then succeeds on 3rd
// attempt. The initial attempt (attempt=0) is not a retry, so the counter
// records 1 failed retry (attempt=1) and 1 successful retry (attempt=2).
func TestDrainer_RetryMetricIncremented(t *testing.T) {
	withShortRetries(t)

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("retry-metric-harness")

	retryAttempts, err := meter.Int64Counter("strait_audit_retry_attempts_total")
	require.NoError(t, err)

	emitted, err := meter.Int64Counter("strait_audit_events_emitted_total")
	require.NoError(t, err)

	var attempts atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			n := attempts.Add(1)
			if n <= 2 {
				return errors.New("transient")
			}
			return nil
		},
	}
	srv := NewServer(ServerDeps{
		Config:  &config.Config{InternalSecret: "test", JWTSigningKey: testJWTSigningKey},
		Store:   ms,
		Metrics: &telemetry.Metrics{AuditRetryAttempts: retryAttempts, AuditEventsEmitted: emitted},
	})
	t.Cleanup(srv.Close)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", nil)
	srv.Close()

	h := &backpressureMetricsHarness{reader: reader}

	successCount := h.sumCounterByAttr(t, "strait_audit_retry_attempts_total", "outcome", "success")
	failedCount := h.sumCounterByAttr(t, "strait_audit_retry_attempts_total", "outcome", "failed")
	assert.EqualValues(t, 1, successCount)
	assert.EqualValues(t, 1, failedCount)
}
