package api

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestEmitAuditEvent_SyncWriteFailure_IncrementsDroppedMetric verifies that
// the synchronous emitAuditEvent path records a dropped-event metric when
// CreateAuditEvent returns an error. Before the fix, the sync path only logged
// a warning — the failure was invisible to metrics-based alerting.
func TestEmitAuditEvent_SyncWriteFailure_IncrementsDroppedMetric(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("sync-dropped-harness")

	dropped, err := meter.Int64Counter("strait_audit_events_dropped_total")
	require.NoError(t, err)

	var calls atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			calls.Add(1)
			return errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	srv.metrics = &telemetry.Metrics{AuditEventsDropped: dropped}

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name": "x", "slug": "x", "execution_mode": "http",
	})
	require.EqualValues(t, 1, calls.
		Load())

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.
			Background(), &rm))

	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if inst.Name != "strait_audit_events_dropped_total" {
				continue
			}
			sum, ok := inst.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				v, present := dp.Attributes.Value(attribute.Key("reason"))
				if present && v.AsString() == "sync_write_failed" {
					total += dp.Value
				}
			}
		}
	}
	assert.EqualValues(t, 1, total)

}

// TestEmitAuditEvent_SyncWriteSuccess_NoDroppedMetric verifies that a
// successful sync write does not increment the dropped-event counter.
func TestEmitAuditEvent_SyncWriteSuccess_NoDroppedMetric(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("sync-success-harness")

	dropped, err := meter.Int64Counter("strait_audit_events_dropped_total")
	require.NoError(t, err)

	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	srv.metrics = &telemetry.Metrics{AuditEventsDropped: dropped}

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name": "x", "slug": "x", "execution_mode": "http",
	})

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.
			Background(), &rm))

	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if inst.Name != "strait_audit_events_dropped_total" {
				continue
			}
			sum, ok := inst.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	assert.EqualValues(t, 0, total)

}
