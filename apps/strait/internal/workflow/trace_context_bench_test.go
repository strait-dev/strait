package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	otelTrace "go.opentelemetry.io/otel/trace"
)

func benchmarkWorkflowTraceContext(tb testing.TB) context.Context {
	tb.Helper()

	traceID, err := otelTrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(tb, err)
	spanID, err := otelTrace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(tb, err)

	return otelTrace.ContextWithSpanContext(context.Background(), otelTrace.NewSpanContext(otelTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: otelTrace.FlagsSampled,
	}))
}

func TestWorkflowTraceContextTraceparentFormat(t *testing.T) {
	t.Parallel()

	traceContext := workflowTraceContext(benchmarkWorkflowTraceContext(t))
	require.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", traceContext["traceparent"])
}

func BenchmarkWorkflowTraceContext(b *testing.B) {
	ctx := benchmarkWorkflowTraceContext(b)

	b.ReportAllocs()
	for b.Loop() {
		traceContext := workflowTraceContext(ctx)
		if len(traceContext) == 0 {
			b.Fatal("missing trace context")
		}
	}
}
