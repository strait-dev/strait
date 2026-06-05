package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestAttachAuditContext_RequestIDOnSpan is the regression test for STR-529.
// The chi RequestID middleware sets a request id on the request context;
// attachAuditContext must in turn stamp that value onto the active OTel span
// so trace explorers can pivot to the matching log line.
func TestAttachAuditContext_RequestIDOnSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	origTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(origTP) })

	srv := &Server{}

	tracer := tp.Tracer("test")
	handler := chimw.RequestID(srv.attachAuditContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracer.Start(r.Context(), "child")
		defer span.End()
		assert.NotEmpty(t, requestIDFromContext(r.
			Context()))

		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rr := httptest.NewRecorder()

	tracer2 := tp.Tracer("outer")
	ctx, parent := tracer2.Start(context.Background(), "parent")

	handler.ServeHTTP(rr, req.WithContext(ctx))
	parent.End()

	spans := exporter.GetSpans()
	require.NotEmpty(t, spans)

	// Find the parent span (the one started here) — that's the active span
	// at the moment attachAuditContext stamps the attribute.
	var parentAttrs []attribute.KeyValue
	for _, s := range spans {
		if s.Name == "parent" {
			parentAttrs = s.Attributes
			break
		}
	}
	require.NotNil(t, parentAttrs)

	found := false
	for _, kv := range parentAttrs {
		require.NotEqual(t, "http.request_id",
			kv.Key)

		if kv.Key == attrRequestID {
			require.NotEmpty(t, kv.
				Value.AsString())

			found = true
		}
	}
	require.True(
		t, found)
}

// TestAttachAuditContext_AttrKeyIsVendorNamespaced is the regression test for
// review item 2: the attribute key must be vendor-namespaced so it cannot
// collide with the OTel semconv `http.request.id` if it graduates.
func TestAttachAuditContext_AttrKeyIsVendorNamespaced(t *testing.T) {
	require.Equal(t, "strait.request_id",
		attrRequestID,
	)
}

// TestAttachAuditContext_NoSpanWhenTracingDisabled verifies that calling
// attachAuditContext without an active span does not panic and does not
// leak attributes to a non-recording span.
func TestAttachAuditContext_NoSpanWhenTracingDisabled(t *testing.T) {
	srv := &Server{}

	called := false
	handler := chimw.RequestID(srv.attachAuditContext(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		assert.NotEmpty(t, requestIDFromContext(r.
			Context()))
	})))

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.True(
		t, called)
}
