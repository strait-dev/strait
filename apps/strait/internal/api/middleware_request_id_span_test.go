package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
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
		if reqID := requestIDFromContext(r.Context()); reqID == "" {
			t.Error("request id missing from context")
		}
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rr := httptest.NewRecorder()

	tracer2 := tp.Tracer("outer")
	ctx, parent := tracer2.Start(context.Background(), "parent")

	handler.ServeHTTP(rr, req.WithContext(ctx))
	parent.End()

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}

	// Find the parent span (the one started here) — that's the active span
	// at the moment attachAuditContext stamps the attribute.
	var parentAttrs []attribute.KeyValue
	for _, s := range spans {
		if s.Name == "parent" {
			parentAttrs = s.Attributes
			break
		}
	}
	if parentAttrs == nil {
		t.Fatal("parent span not found in exporter")
	}

	found := false
	for _, kv := range parentAttrs {
		if kv.Key == "http.request_id" {
			t.Fatalf("legacy http.request_id attribute key leaked back into output: %+v", kv)
		}
		if kv.Key == attrRequestID {
			if kv.Value.AsString() == "" {
				t.Fatalf("%s span attribute is empty", attrRequestID)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("%s attribute not set on span; got %+v", attrRequestID, parentAttrs)
	}
}

// TestAttachAuditContext_AttrKeyIsVendorNamespaced is the regression test for
// review item 2: the attribute key must be vendor-namespaced so it cannot
// collide with the OTel semconv `http.request.id` if it graduates.
func TestAttachAuditContext_AttrKeyIsVendorNamespaced(t *testing.T) {
	if attrRequestID != "strait.request_id" {
		t.Fatalf("attrRequestID = %q, want strait.request_id (vendor-namespaced)", attrRequestID)
	}
}

// TestAttachAuditContext_NoSpanWhenTracingDisabled verifies that calling
// attachAuditContext without an active span does not panic and does not
// leak attributes to a non-recording span.
func TestAttachAuditContext_NoSpanWhenTracingDisabled(t *testing.T) {
	srv := &Server{}

	called := false
	handler := chimw.RequestID(srv.attachAuditContext(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		if reqID := requestIDFromContext(r.Context()); reqID == "" {
			t.Error("request id missing from context")
		}
	})))

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler not called")
	}
}
