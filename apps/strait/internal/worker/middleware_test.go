package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Chain() tests.

func TestChain_Empty(t *testing.T) {
	t.Parallel()
	called := false
	handler := func(_ context.Context, _ *ExecutionContext) { called = true }

	wrapped := Chain()(handler)
	wrapped(context.Background(), &ExecutionContext{})

	if !called {
		t.Fatal("handler was not called with empty chain")
	}
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
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("at index %d: expected %q, got %q", i, expected[i], order[i])
		}
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
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("at index %d: expected %q, got %q", i, expected[i], order[i])
		}
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

	if handlerCalled {
		t.Fatal("handler should not have been called after short-circuit")
	}
	if cReached {
		t.Fatal("middleware C should not have been reached after short-circuit in B")
	}
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

	if gotValue != "injected" {
		t.Fatalf("expected context value %q, got %v", "injected", gotValue)
	}
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

	if gotJob != injectedJob {
		t.Fatal("handler did not see the job injected by middleware")
	}
}

// --- TracingMiddleware tests ---
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
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}
		if spans[0].Name != "executor.Execute" {
			t.Fatalf("expected span name %q, got %q", "executor.Execute", spans[0].Name)
		}
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
			Job:   &domain.Job{EndpointURL: "https://api.example.com/hook", Version: 3},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		attrs := spans[0].Attributes
		assertAttr(t, attrs, "job.endpoint", "https://api.example.com/hook")
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
			if attr.Key == "job.endpoint" || attr.Key == "job.version" {
				t.Fatalf("unexpected job attribute %q when Job is nil", attr.Key)
			}
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
		if len(spans) == 0 {
			t.Fatal("expected at least 1 span")
		}
		// The span should inherit the trace ID from the injected traceparent.
		gotTraceID := spans[0].SpanContext.TraceID().String()
		if gotTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("expected trace ID from traceparent, got %s", gotTraceID)
		}
	})

	t.Run("NilMetadata_NoPanic", func(t *testing.T) {
		exporter.Reset()
		ec := &ExecutionContext{
			Run:   &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Attempt: 1, Metadata: nil},
			Start: time.Now(),
		}
		TracingMiddleware()(nopHandler)(context.Background(), ec)

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span even with nil metadata, got %d", len(spans))
		}
	})
}

// Helpers.

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key, expected string) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if a.Value.AsString() != expected {
				t.Fatalf("attribute %q: expected %q, got %q", key, expected, a.Value.AsString())
			}
			return
		}
	}
	t.Fatalf("attribute %q not found", key)
}

func assertAttrInt(t *testing.T, attrs []attribute.KeyValue, key string, expected int) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if a.Value.AsInt64() != int64(expected) {
				t.Fatalf("attribute %q: expected %d, got %d", key, expected, a.Value.AsInt64())
			}
			return
		}
	}
	t.Fatalf("attribute %q not found", key)
}
