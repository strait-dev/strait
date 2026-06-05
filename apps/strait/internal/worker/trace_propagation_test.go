package worker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// TestTraceChain groups end-to-end trace chain tests that share global OTel state.
func TestTraceChain(t *testing.T) {
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

	t.Run("WorkflowToHTTPDispatch", func(t *testing.T) {
		exporter.Reset()

		// Create a span to get a known trace ID.
		_, parentSpan := tp.Tracer("test").Start(context.Background(), "workflow.run")
		sc := parentSpan.SpanContext()
		traceID := sc.TraceID().String()
		spanID := sc.SpanID().String()
		traceparent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)
		parentSpan.End()

		// Build a JobRun with the traceparent in metadata (simulating engine_steps.go propagation).
		run := &domain.JobRun{
			ID:        "run-chain-1",
			JobID:     "job-chain-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata: map[string]string{
				"_trace_parent": traceparent,
			},
		}

		// Run TracingMiddleware and verify the span inherits the trace ID.
		exporter.Reset()
		mw := TracingMiddleware()
		var innerCtx context.Context
		handler := mw(func(ctx context.Context, _ *ExecutionContext) {
			innerCtx = ctx //nolint:fatcontext // test captures ctx for assertion
		})
		handler(context.Background(), &ExecutionContext{Run: run, Job: &domain.Job{EndpointURL: "http://example.com"}})

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		mwSpan := spans[len(spans)-1]
		assert.Equal(t,
			traceID,
			mwSpan.SpanContext.
				TraceID().String())

		// Verify that the inner context carries the correct trace.
		innerSC := otelTrace.SpanFromContext(innerCtx).SpanContext()
		assert.Equal(t,
			traceID,
			innerSC.TraceID().String())

		// Verify dispatchToEndpoint sets the Traceparent header with the same trace ID.
		var mu sync.Mutex
		var capturedHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			capturedHeaders = r.Header.Clone()
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		e := &Executor{httpClient: srv.Client()}
		_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
		require.NoError(
			t, err)

		mu.Lock()
		gotTP := capturedHeaders.Get("Traceparent")
		mu.Unlock()
		assert.True(t, strings.Contains(gotTP,
			traceID))

	})

	t.Run("NoTraceContext_NoLeaks", func(t *testing.T) {
		exporter.Reset()

		// No _trace_parent in metadata.
		run := &domain.JobRun{
			ID:        "run-noleak-1",
			JobID:     "job-noleak-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata:  nil,
		}

		// TracingMiddleware should create a root span without panic.
		mw := TracingMiddleware()
		handler := mw(func(_ context.Context, _ *ExecutionContext) {})
		handler(context.Background(), &ExecutionContext{Run: run, Job: &domain.Job{EndpointURL: "http://example.com"}})

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// The span should be a root (no remote parent).
		mwSpan := spans[len(spans)-1]
		assert.False(t,
			mwSpan.
				Parent.IsValid() && mwSpan.Parent.
				IsRemote(),
		)

		// dispatchToEndpoint should NOT set Traceparent header when metadata has no _trace_parent.
		var mu sync.Mutex
		var capturedHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			capturedHeaders = r.Header.Clone()
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		e := &Executor{httpClient: srv.Client()}
		_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
		require.NoError(
			t, err)

		mu.Lock()
		gotTP := capturedHeaders.Get("Traceparent")
		mu.Unlock()
		assert.Equal(t,
			"", gotTP,
		)

	})

	t.Run("SpanParentChild", func(t *testing.T) {
		exporter.Reset()

		// Create a parent span with a known span ID.
		_, parentSpan := tp.Tracer("test").Start(context.Background(), "parent.span")
		sc := parentSpan.SpanContext()
		traceID := sc.TraceID().String()
		parentSpanID := sc.SpanID().String()
		traceparent := fmt.Sprintf("00-%s-%s-01", traceID, parentSpanID)
		parentSpan.End()

		run := &domain.JobRun{
			ID:        "run-parent-1",
			JobID:     "job-parent-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata: map[string]string{
				"_trace_parent": traceparent,
			},
		}

		exporter.Reset()
		mw := TracingMiddleware()
		handler := mw(func(_ context.Context, _ *ExecutionContext) {})
		handler(context.Background(), &ExecutionContext{Run: run, Job: &domain.Job{EndpointURL: "http://example.com"}})

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		mwSpan := spans[len(spans)-1]
		assert.Equal(t,
			parentSpanID,
			mwSpan.
				Parent.SpanID().String())
		assert.Equal(t,
			traceID,
			mwSpan.SpanContext.
				TraceID().String())

		// The created span's parent should have the span ID from the traceparent.

		// The span should share the same trace ID.

	})

	t.Run("MultipleWorkflowSteps", func(t *testing.T) {
		exporter.Reset()

		// Create a single traceparent (simulating one workflow run).
		_, parentSpan := tp.Tracer("test").Start(context.Background(), "workflow.run.multi")
		sc := parentSpan.SpanContext()
		traceID := sc.TraceID().String()
		spanID := sc.SpanID().String()
		traceparent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)
		parentSpan.End()

		// Create 3 separate JobRuns with the same _trace_parent.
		runs := []*domain.JobRun{
			{ID: "run-multi-1", JobID: "job-multi-1", ProjectID: "proj-1", Attempt: 1, Metadata: map[string]string{"_trace_parent": traceparent}},
			{ID: "run-multi-2", JobID: "job-multi-2", ProjectID: "proj-1", Attempt: 1, Metadata: map[string]string{"_trace_parent": traceparent}},
			{ID: "run-multi-3", JobID: "job-multi-3", ProjectID: "proj-1", Attempt: 1, Metadata: map[string]string{"_trace_parent": traceparent}},
		}

		exporter.Reset()
		mw := TracingMiddleware()
		for _, run := range runs {
			handler := mw(func(_ context.Context, _ *ExecutionContext) {})
			handler(context.Background(), &ExecutionContext{Run: run, Job: &domain.Job{EndpointURL: "http://example.com"}})
		}

		spans := exporter.GetSpans()
		require.Len(t, spans,
			3,
		)

		// All 3 spans should have the same trace ID.
		for _, s := range spans {
			assert.Equal(t,
				traceID,
				s.SpanContext.
					TraceID().String())

		}

		// All 3 spans should have DIFFERENT span IDs.
		seen := make(map[string]bool)
		for _, s := range spans {
			sid := s.SpanContext.SpanID().String()
			assert.False(t,
				seen[sid])

			seen[sid] = true
		}
	})
}

// TestTraceAdversarial groups adversarial and security tests that share global OTel state.
func TestTraceAdversarial(t *testing.T) {
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

	t.Run("HeaderInjectionAttempt", func(t *testing.T) {
		injectedValue := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01\r\nX-Evil: injected"

		run := &domain.JobRun{
			ID:        "run-inject-1",
			JobID:     "job-inject-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata: map[string]string{
				"_trace_parent": injectedValue,
			},
		}

		var mu sync.Mutex
		var capturedHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			capturedHeaders = r.Header.Clone()
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		e := &Executor{httpClient: srv.Client()}
		_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)

		// Go's net/http rejects header values containing \r\n entirely,
		// so either the request fails (safe) or the header is sanitized (also safe).
		if err != nil {
			require.True(t,
				strings.Contains(err.
					Error(), "invalid header",
				))

			// The request was rejected before being sent -- CRLF injection prevented.

			return
		}

		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t,
			"", capturedHeaders.
				Get("X-Evil"))

		// If the request somehow went through, X-Evil must NOT appear.

	})

	t.Run("OverlongTraceparent", func(t *testing.T) {
		exporter.Reset()

		// 10KB string as traceparent.
		overlong := strings.Repeat("a", 10*1024)

		run := &domain.JobRun{
			ID:        "run-overlong-1",
			JobID:     "job-overlong-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata: map[string]string{
				"_trace_parent": overlong,
			},
		}

		// TracingMiddleware should not panic; OTel ignores invalid traceparent
		// and falls back to creating a root span.
		mw := TracingMiddleware()
		handler := mw(func(_ context.Context, _ *ExecutionContext) {})
		handler(context.Background(), &ExecutionContext{Run: run, Job: &domain.Job{EndpointURL: "http://example.com"}})

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans)

		// The span should be a root (OTel should reject the invalid traceparent).
		mwSpan := spans[len(spans)-1]
		assert.False(t,
			mwSpan.
				Parent.IsValid() && mwSpan.Parent.
				IsRemote(),
		)

		// dispatchToEndpoint should not panic with overlong value.
		var mu sync.Mutex
		var capturedHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			capturedHeaders = r.Header.Clone()
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		e := &Executor{httpClient: srv.Client()}
		_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
		require.NoError(
			t, err)

		mu.Lock()
		gotTP := capturedHeaders.Get("Traceparent")
		mu.Unlock()
		assert.Equal(t,
			overlong,
			gotTP)

		// The header is set (HTTP allows long headers); just verify no panic occurred.

	})

	t.Run("NullBytesInMetadata", func(t *testing.T) {
		// Null bytes in strings: Go's json.Marshal escapes them, and
		// http.Header.Set should not panic.
		valueWithNull := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01\x00evil"

		run := &domain.JobRun{
			ID:        "run-null-1",
			JobID:     "job-null-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata: map[string]string{
				"_trace_parent": valueWithNull,
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		e := &Executor{httpClient: srv.Client()}
		// Should not panic regardless of outcome.
		_, _ = e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)

		// If we got here without panic, the test passes.
	})

	t.Run("InternalKeysNotExposedViaAPI", func(t *testing.T) {
		// Verify that dispatchToEndpoint only sets Traceparent and Tracestate
		// from metadata, and no other internal _-prefixed keys leak as headers.
		run := &domain.JobRun{
			ID:        "run-leak-1",
			JobID:     "job-leak-1",
			ProjectID: "proj-1",
			Attempt:   1,
			Metadata: map[string]string{
				"_trace_parent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				"_trace_state":  "congo=t61rcWkgMzE",
				"_internal_key": "should-not-appear",
				"user_key":      "should-not-appear-either",
			},
		}

		var mu sync.Mutex
		var capturedHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			capturedHeaders = r.Header.Clone()
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()

		e := &Executor{httpClient: srv.Client()}
		_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
		require.NoError(
			t, err)

		mu.Lock()
		defer mu.Unlock()
		assert.NotEqual(
			t, "",
			capturedHeaders.
				Get("Traceparent"))
		assert.NotEqual(
			t, "",
			capturedHeaders.
				Get("Tracestate"),
		)

		// Traceparent and Tracestate should be present.

		// No other metadata keys should appear as headers.
		// Check that _internal_key and user_key did not leak.
		for key := range capturedHeaders {
			lower := strings.ToLower(key)
			assert.False(t,
				lower ==
					"_internal_key" ||
					lower == "user_key" ||
					lower ==
						"x-internal-key" ||
					lower == "x-user-key",
			)

		}

		// Verify only expected headers are present (Content-Type, X-Run-ID, X-Job-ID,
		// X-Attempt, Traceparent, Tracestate, plus standard Go HTTP headers).
		allowedHeaders := map[string]bool{
			"Content-Type":    true,
			"X-Run-Id":        true,
			"X-Job-Id":        true,
			"X-Attempt":       true,
			"Traceparent":     true,
			"Tracestate":      true,
			"Content-Length":  true,
			"User-Agent":      true,
			"Accept-Encoding": true,
			"Host":            true,
		}
		for key := range capturedHeaders {
			assert.True(t, allowedHeaders[key])

		}
	})
}
