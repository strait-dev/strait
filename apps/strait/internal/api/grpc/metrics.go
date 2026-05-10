package grpc

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var grpcMetrics = newWorkerPlaneMetrics()

type workerPlaneMetrics struct {
	streamsOpen       metric.Int64UpDownCounter
	streamDisconnects metric.Int64Counter
	dispatchE2E       metric.Float64Histogram
}

func newWorkerPlaneMetrics() workerPlaneMetrics {
	meter := otel.Meter("strait/grpc_worker")
	streamsOpen, _ := meter.Int64UpDownCounter(
		"strait_grpc_worker_streams_open",
		metric.WithDescription("Open gRPC worker streams by queue"),
		metric.WithUnit("1"),
	)
	streamDisconnects, _ := meter.Int64Counter(
		"strait_grpc_worker_stream_disconnects_total",
		metric.WithDescription("gRPC worker stream disconnects by reason"),
		metric.WithUnit("1"),
	)
	dispatchE2E, _ := meter.Float64Histogram(
		"strait_grpc_run_dispatch_e2e_seconds",
		metric.WithDescription("End-to-end gRPC worker dispatch latency from assignment to result"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60),
	)
	return workerPlaneMetrics{streamsOpen: streamsOpen, streamDisconnects: streamDisconnects, dispatchE2E: dispatchE2E}
}

func recordWorkerStreamsOpen(ctx context.Context, queues []string, delta int64) {
	if len(queues) == 0 {
		queues = []string{"default"}
	}
	for _, queue := range queues {
		if queue == "" {
			queue = "default"
		}
		grpcMetrics.streamsOpen.Add(ctx, delta, metric.WithAttributes(attribute.String("queue", queue)))
	}
}

func recordWorkerStreamDisconnect(ctx context.Context, reason string) {
	if reason == "" {
		reason = "unknown"
	}
	grpcMetrics.streamDisconnects.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func recordGRPCDispatchE2E(ctx context.Context, started time.Time) {
	if started.IsZero() {
		return
	}
	grpcMetrics.dispatchE2E.Record(ctx, time.Since(started).Seconds())
}
