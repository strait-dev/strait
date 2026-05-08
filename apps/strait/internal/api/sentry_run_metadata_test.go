package api

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"

	"strait/internal/domain"
)

func TestSentryRunMetadataCarriesTraceContext(t *testing.T) {
	t.Parallel()

	traceID := trace.TraceID{1, 2, 3}
	spanID := trace.SpanID{4, 5, 6}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx = sentry.SetHubOnContext(ctx, hub)

	got := sentryRunMetadata(ctx, "POST /v1/jobs/{jobID}/trigger", map[string]string{
		"dependency_key": "dep-1",
	})
	if got["dependency_key"] != "dep-1" {
		t.Fatalf("dependency_key = %q, want dep-1", got["dependency_key"])
	}
	if got[domain.RunMetadataSentryTrace] == "" {
		t.Fatal("expected Sentry trace metadata")
	}
	wantTraceparent := "00-" + traceID.String() + "-" + spanID.String() + "-01"
	if got[domain.RunMetadataTraceParent] != wantTraceparent {
		t.Fatalf("traceparent = %q, want %q", got[domain.RunMetadataTraceParent], wantTraceparent)
	}
}

func TestApplyRunTraceHeaderMetadataOverridesContextTrace(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{
		domain.RunMetadataSentryTrace: "context-sentry-trace",
		domain.RunMetadataTraceParent: "context-traceparent",
	}
	got := applyRunTraceHeaderMetadata(
		metadata,
		"00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		"congo=t61rcWkgMzE",
		"0123456789abcdef0123456789abcdef-0123456789abcdef-1",
		"sentry-release=test-release,sentry-public_key=public",
	)

	if got[domain.RunMetadataTraceParent] != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Fatalf("traceparent = %q, want explicit header", got[domain.RunMetadataTraceParent])
	}
	if got[domain.RunMetadataTraceState] != "congo=t61rcWkgMzE" {
		t.Fatalf("tracestate = %q, want explicit header", got[domain.RunMetadataTraceState])
	}
	if got[domain.RunMetadataSentryTrace] != "0123456789abcdef0123456789abcdef-0123456789abcdef-1" {
		t.Fatalf("sentry trace = %q, want explicit header", got[domain.RunMetadataSentryTrace])
	}
	if got[domain.RunMetadataSentryBaggage] != "sentry-release=test-release,sentry-public_key=public" {
		t.Fatalf("baggage = %q, want explicit header", got[domain.RunMetadataSentryBaggage])
	}
}
