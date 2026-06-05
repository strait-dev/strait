package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Chain() tests.

func TestChain_Empty(t *testing.T) {
	t.Parallel()
	called := false
	handler := func(_ context.Context, _ *ExecutionContext) { called = true }

	wrapped := Chain()(handler)
	wrapped(context.Background(), &ExecutionContext{})
	require.True(t,
		called)
}

func TestChain_Single(t *testing.T) {
	t.Parallel()
	var order []string

	mw := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			order = append(order, "A-before")
			next(ctx, ec)
			order = append(order, "A-after")
		}
	}
	handler := func(_ context.Context, _ *ExecutionContext) {
		order = append(order, "handler")
	}

	Chain(mw)(handler)(context.Background(), &ExecutionContext{})

	expected := []string{"A-before", "handler", "A-after"}
	require.Len(t, order,
		len(expected),
	)

	for i := range expected {
		require.Equal(t,
			expected[i], order[i])
	}
}

func TestChain_Multiple_OnionOrder(t *testing.T) {
	t.Parallel()
	var order []string

	makeMW := func(name string) ExecutionMiddleware {
		return func(next ExecutionHandler) ExecutionHandler {
			return func(ctx context.Context, ec *ExecutionContext) {
				order = append(order, name+"-before")
				next(ctx, ec)
				order = append(order, name+"-after")
			}
		}
	}
	handler := func(_ context.Context, _ *ExecutionContext) {
		order = append(order, "handler")
	}

	Chain(makeMW("A"), makeMW("B"), makeMW("C"))(handler)(context.Background(), &ExecutionContext{})

	expected := []string{"A-before", "B-before", "C-before", "handler", "C-after", "B-after", "A-after"}
	require.Len(t, order,
		len(expected),
	)

	for i := range expected {
		require.Equal(t,
			expected[i], order[i])
	}
}

func TestChain_ShortCircuit(t *testing.T) {
	t.Parallel()
	handlerCalled := false
	cReached := false

	mwA := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			next(ctx, ec) // calls B
		}
	}
	mwB := func(_ ExecutionHandler) ExecutionHandler {
		return func(_ context.Context, _ *ExecutionContext) {
			// Does NOT call next -- short-circuits
		}
	}
	mwC := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			cReached = true
			next(ctx, ec)
		}
	}
	handler := func(_ context.Context, _ *ExecutionContext) {
		handlerCalled = true
	}

	Chain(mwA, mwB, mwC)(handler)(context.Background(), &ExecutionContext{})
	require.False(t,
		handlerCalled,
	)
	require.False(t,
		cReached)
}

type ctxKey string

func TestChain_ContextPropagation(t *testing.T) {
	t.Parallel()
	key := ctxKey("test-key")
	var gotValue any

	mw := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			ctx = context.WithValue(ctx, key, "injected")
			next(ctx, ec)
		}
	}
	handler := func(ctx context.Context, _ *ExecutionContext) {
		gotValue = ctx.Value(key)
	}

	Chain(mw)(handler)(context.Background(), &ExecutionContext{})
	require.Equal(t,
		"injected",
		gotValue,
	)
}

func TestChain_ExecutionContextModification(t *testing.T) {
	t.Parallel()
	var gotJob *domain.Job
	injectedJob := &domain.Job{ID: "job-injected", EndpointURL: "https://example.com"}

	mw := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			ec.Job = injectedJob
			next(ctx, ec)
		}
	}
	handler := func(_ context.Context, ec *ExecutionContext) {
		gotJob = ec.Job
	}

	Chain(mw)(handler)(context.Background(), &ExecutionContext{
		Run: &domain.JobRun{ID: "run-1"},
	})
	require.Equal(t,
		injectedJob,
		gotJob,
	)
}

// These tests share a global TracerProvider, so they run sequentially under a
// single parent test to avoid races on the global OTel state.

func TestTracingMiddleware(t *testing.T) {
	// Do NOT call t.Parallel() — these subtests mutate global OTel state.

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})

	nopHandler := func(_ context.Context, _ *ExecutionContext) {}

	t.Run("CreatesSpan", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run:   &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.Len(t, spans,
			1)
		require.Equal(t,
			"executor.Execute",

			spans[0].Name)
	})

	t.Run("SetsRunAttributes", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run:   &domain.JobRun{ID: "run-42", JobID: "job-7", ProjectID: "proj-3", Attempt: 2},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		attrs := spans[0].Attributes
		assertAttr(t, attrs, "run.id", "run-42")
		assertAttr(t, attrs, "job.id", "job-7")
		assertAttr(t, attrs, "project.id", "proj-3")
		assertAttrInt(t, attrs, "run.attempt", 2)
	})

	t.Run("SetsJobAttributes", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run:   &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1},
			Job:   &domain.Job{EndpointURL: "https://user:pass@api.example.com/hook?token=secret#frag", Version: 3},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		attrs := spans[0].Attributes
		assertAttr(t, attrs, "job.endpoint", "https://api.example.com")
		assertAttrInt(t, attrs, "job.version", 3)
	})

	t.Run("NoJobAttributes_NilJob", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run:   &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1},
			Job:   nil,
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		for _, attr := range spans[0].Attributes {
			require.False(t,
				attr.Key ==
					"job.endpoint" ||
					attr.Key == "job.version",
			)
		}
	})

	t.Run("ExtractsTraceParent", func(t *testing.T) {
		exporter.Reset()
		traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{"_trace_parent": traceParent},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// The span should inherit the trace ID from the injected traceparent.
		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.Equal(t,
			"4bf92f3577b34da6a3ce929d0e0e4736",

			gotTraceID,
		)
	})

	t.Run("NilMetadata_NoPanic", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run:   &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1, Metadata: nil},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.Len(t, spans,
			1)
	})

	t.Run("ExtractsTraceParentAndState", func(t *testing.T) {
		exporter.Reset()
		traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		traceState := "congo=t61rcWkgMzE"

		var capturedCtx context.Context
		capturingHandler := func(ctx context.Context, _ *ExecutionContext) {
			capturedCtx = ctx //nolint:fatcontext // test captures ctx for assertion
		}

		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{
					"_trace_parent": traceParent,
					"_trace_state":  traceState,
				},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(capturingHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.Equal(t,
			"4bf92f3577b34da6a3ce929d0e0e4736",

			gotTraceID,
		)

		// Re-extract tracestate from the context the handler received.
		carrier := propagation.MapCarrier{}
		otel.GetTextMapPropagator().Inject(capturedCtx, carrier)
		gotTS := carrier.Get("tracestate")
		require.Equal(t,
			traceState,
			gotTS)
	})

	t.Run("EmptyTraceParent_Ignored", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{"_trace_parent": ""},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// An empty traceparent should not inject a parent; the span should be a root
		// with a freshly generated trace ID (not matching any injected value).
		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.NotEqual(t, "4bf92f3577b34da6a3ce929d0e0e4736",

			gotTraceID,
		)
	})

	t.Run("MalformedTraceParent_Graceful", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{"_trace_parent": "not-a-valid-traceparent"},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// Malformed traceparent is ignored by OTel; span should be root.
		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.NotEqual(t, "4bf92f3577b34da6a3ce929d0e0e4736",

			gotTraceID,
		)
	})

	t.Run("AllZerosTraceID_Ignored", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{
					"_trace_parent": "00-00000000000000000000000000000000-0000000000000000-01",
				},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// All-zeros trace ID is invalid per W3C; span should be root.
		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.NotEqual(t, "00000000000000000000000000000000",

			gotTraceID,
		)
	})

	t.Run("SampledFlagZero_StillPropagates", func(t *testing.T) {
		exporter.Reset()
		// sampled=0 (last byte 00), but trace ID should still be inherited.
		traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"

		var capturedCtx context.Context
		capturingHandler := func(ctx context.Context, _ *ExecutionContext) {
			capturedCtx = ctx //nolint:fatcontext // test captures ctx for assertion
		}

		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{"_trace_parent": traceParent},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(capturingHandler)(context.Background(), ec)

		// The default ParentBased sampler respects sampled=0, so the span may
		// not be exported. Instead, verify via the context that the trace ID
		// was inherited from the parent.
		sc := oteltrace.SpanFromContext(capturedCtx).SpanContext()
		gotTraceID := sc.TraceID().String()
		require.Equal(t,
			"4bf92f3577b34da6a3ce929d0e0e4736",

			gotTraceID,
		)
	})

	t.Run("OnlyTraceState_NoParent", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{"_trace_state": "congo=t61rcWkgMzE"},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// Without _trace_parent, the middleware should not inject any parent context.
		// The span should be a root with a fresh trace ID.
		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.NotEqual(t, "00000000000000000000000000000000",

			gotTraceID,
		)
	})

	t.Run("MetadataWithNonTraceKeys", func(t *testing.T) {
		exporter.Reset()
		traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		ec := &ExecutionContext{
			Run: &domain.JobRun{
				ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1,
				Metadata: map[string]string{
					"foo":           "bar",
					"_trace_parent": traceParent,
				},
			},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		gotTraceID := spans[0].SpanContext.TraceID().String()
		require.Equal(t,
			"4bf92f3577b34da6a3ce929d0e0e4736",

			gotTraceID,
		)
	})
}

// Helpers.

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key, expected string) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			require.Equal(t,
				expected,
				a.Value.
					AsString())

			return
		}
	}
	require.Failf(t, "test failure",

		"attribute %q not found", key)
}

func assertAttrInt(t *testing.T, attrs []attribute.KeyValue, key string, expected int) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			require.Equal(t,
				int64(expected), a.
					Value.AsInt64())

			return
		}
	}
	require.Failf(t, "test failure",

		"attribute %q not found", key)
}
