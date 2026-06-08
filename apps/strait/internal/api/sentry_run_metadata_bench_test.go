package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"strait/internal/domain"
)

func benchmarkTraceContext(tb testing.TB) context.Context {
	tb.Helper()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(tb, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(tb, err)

	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}

func TestApplyTraceContextMetadataTraceparentFormat(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{}
	applyTraceContextMetadata(benchmarkTraceContext(t), metadata)
	require.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", metadata[domain.RunMetadataTraceParent])
}

func BenchmarkApplyTraceContextMetadata(b *testing.B) {
	ctx := benchmarkTraceContext(b)
	metadata := make(map[string]string, 1)

	b.ReportAllocs()
	for b.Loop() {
		clear(metadata)
		applyTraceContextMetadata(ctx, metadata)
		if metadata[domain.RunMetadataTraceParent] == "" {
			b.Fatal("missing traceparent")
		}
	}
}
