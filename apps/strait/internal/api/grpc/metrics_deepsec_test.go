package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestDeepSecWorkerQueueMetricKindDoesNotExposeQueueNames(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":                    "default",
		"default":             "default",
		"billing-prod":        "custom",
		"tenant-secret-queue": "custom",
	}
	for queue, want := range cases {
		require.Equal(t, want, workerQueueMetricKind(queue))
	}
}

func TestWorkerPlaneMetricsRecordBoundedLabels(t *testing.T) {
	reader := setupWorkerPlaneMetricsReader(t)

	recordWorkerStreamsOpen(context.Background(), nil, 1)
	recordWorkerStreamsOpen(context.Background(), []string{"default", "tenant-secret-queue"}, -1)
	recordWorkerStreamDisconnect(context.Background(), "")
	recordGRPCDispatchE2E(context.Background(), time.Time{})
	require.EqualValues(t, 0, grpcDispatchHistogramCount(t, reader))

	recordGRPCDispatchE2E(context.Background(), time.Now().Add(-time.Millisecond))

	require.EqualValues(t, 0, workerStreamsOpenValue(t, reader, "default"))
	require.EqualValues(t, -1, workerStreamsOpenValue(t, reader, "custom"))
	require.EqualValues(t, 1, workerStreamDisconnectValue(t, reader, "unknown"))
	require.EqualValues(t, 1, grpcDispatchHistogramCount(t, reader))
}

func setupWorkerPlaneMetricsReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	oldProvider := otel.GetMeterProvider()
	oldMetrics := grpcMetrics

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	grpcMetrics = newWorkerPlaneMetrics()

	t.Cleanup(func() {
		grpcMetrics = oldMetrics
		otel.SetMeterProvider(oldProvider)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return reader
}

func workerStreamsOpenValue(t *testing.T, reader *sdkmetric.ManualReader, queueKind string) int64 {
	t.Helper()
	data := collectWorkerPlaneSum(t, reader, "strait_grpc_worker_streams_open")
	var total int64
	for _, dp := range data.DataPoints {
		if workerPlaneMetricAttrEq(dp.Attributes, "queue_kind", queueKind) {
			total += dp.Value
		}
	}
	return total
}

func workerStreamDisconnectValue(t *testing.T, reader *sdkmetric.ManualReader, reason string) int64 {
	t.Helper()
	data := collectWorkerPlaneSum(t, reader, "strait_grpc_worker_stream_disconnects_total")
	var total int64
	for _, dp := range data.DataPoints {
		if workerPlaneMetricAttrEq(dp.Attributes, "reason", reason) {
			total += dp.Value
		}
	}
	return total
}

func grpcDispatchHistogramCount(t *testing.T, reader *sdkmetric.ManualReader) uint64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			if metric.Name != "strait_grpc_run_dispatch_e2e_seconds" {
				continue
			}
			data, ok := metric.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			var total uint64
			for _, dp := range data.DataPoints {
				total += dp.Count
			}
			return total
		}
	}
	return 0
}

func collectWorkerPlaneSum(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Sum[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			if metric.Name != name {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			return data
		}
	}
	return metricdata.Sum[int64]{}
}

func workerPlaneMetricAttrEq(set attribute.Set, key, want string) bool {
	got, ok := set.Value(attribute.Key(key))
	return ok && got.AsString() == want
}
