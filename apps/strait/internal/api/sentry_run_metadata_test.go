package api

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, "dep-1", got["dependency_key"])
	require.NotEmpty(t, got[domain.RunMetadataSentryTrace])

	wantTraceparent := "00-" + traceID.String() + "-" + spanID.String() + "-01"
	require.Equal(t, wantTraceparent, got[domain.RunMetadataTraceParent])
	require.True(t, validRunTraceSpanContext(trace.SpanContextFromContext(ctx)))
}

func TestSentryRunMetadataSkipsInvalidTraceContext(t *testing.T) {
	t.Parallel()

	got := sentryRunMetadata(context.Background(), "POST /v1/jobs/{jobID}/trigger", nil)
	require.NotContains(t, got, domain.RunMetadataTraceParent)
	require.False(t, validRunTraceSpanContext(trace.SpanContextFromContext(context.Background())))
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
	require.Equal(t, "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01", got[domain.RunMetadataTraceParent])
	require.Equal(t, "congo=t61rcWkgMzE", got[domain.RunMetadataTraceState])
	require.Equal(t, "0123456789abcdef0123456789abcdef-0123456789abcdef-1", got[domain.RunMetadataSentryTrace])
	require.Equal(t, "sentry-release=test-release,sentry-public_key=public", got[domain.RunMetadataSentryBaggage])
}
