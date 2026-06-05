package grpc

import (
	"context"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"strait/internal/domain"
	"strait/internal/telemetry"
)

func TestGRPCSentryScope_AttachesAPIKeyProjectWorkerAndTrace(t *testing.T) {
	t.Parallel()

	traceID := trace.TraceID{1, 2, 3}
	spanID := trace.SpanID{4, 5, 6}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	}))
	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx = sentry.SetHubOnContext(ctx, hub)
	ctx = withAPIKeyContext(ctx, &domain.APIKey{
		ID:            "key-1",
		ProjectID:     "proj-1",
		OrgID:         "org-1",
		EnvironmentID: "env-1",
	})
	configureGRPCSentryScope(ctx, grpcSentryMetadata{
		edition: string(domain.BuildEdition()),
		mode:    "all",
		region:  "iad",
		version: "test-version",
	}, map[telemetry.SentryTag]string{
		telemetry.TagService: "strait.worker.v1.WorkerService",
		telemetry.TagRPC:     "StreamTasks",
	})
	configureGRPCSentryWorkerScope(ctx, "worker-1", "Worker One", "host-a", "go", "1.2.3")

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t, event)

	wantTags := map[string]string{
		"project_id":     "proj-1",
		"org_id":         "org-1",
		"api_key_id":     "key-1",
		"actor_id":       "apikey:key-1",
		"actor_type":     "api_key",
		"environment_id": "env-1",
		"edition":        string(domain.BuildEdition()),
		"subsystem":      "grpc",
		"mode":           "all",
		"region":         "iad",
		"version":        "test-version",
		"service":        "strait.worker.v1.WorkerService",
		"rpc":            "StreamTasks",
		"worker_id":      "worker-1",
		"worker_name":    "Worker One",
		"worker_host":    "host-a",
		"sdk_language":   "go",
		"sdk_version":    "1.2.3",
		"trace_id":       traceID.String(),
		"span_id":        spanID.String(),
	}
	for key, want := range wantTags {
		require.Equal(
			t, want, event.
				Tags[key])

	}
	require.Equal(
		t, "apikey:key-1",
		event.
			User.ID)
	require.Equal(
		t, "proj-1",
		event.Contexts["grpc.request"]["project_id"],
	)

}

func TestStreamSentryInterceptorSetsHubOnWrappedContext(t *testing.T) {
	t.Parallel()

	interceptor := streamSentryInterceptor(grpcSentryMetadata{})
	info := &grpc.StreamServerInfo{FullMethod: "/strait.worker.v1.WorkerService/StreamTasks"}
	stream := &mockServerStream{ctx: context.Background()}
	var sawHub bool
	err := interceptor(nil, stream, info, func(_ any, ss grpc.ServerStream) error {
		sawHub = sentry.GetHubFromContext(ss.Context()) != nil
		return status.Error(codes.InvalidArgument, "bad worker message")
	})
	require.Error(
		t, err)
	require.True(t,
		sawHub)

}

func TestGRPCSentryScopeContinuesIncomingSentryTrace(t *testing.T) {
	t.Parallel()

	const sentryTrace = "0123456789abcdef0123456789abcdef-0123456789abcdef-1"
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		sentry.SentryTraceHeader, sentryTrace,
		sentry.SentryBaggageHeader, "sentry-release=test-release,sentry-public_key=public",
	))
	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx = sentry.SetHubOnContext(ctx, hub)

	configureGRPCSentryScope(ctx, grpcSentryMetadata{}, map[telemetry.SentryTag]string{
		telemetry.TagService: "strait.worker.v1.WorkerService",
		telemetry.TagRPC:     "StreamTasks",
	})

	traceparent := hub.GetTraceparent()
	require.True(t,
		strings.Contains(traceparent,

			"0123456789abcdef0123456789abcdef",
		))

	if baggage := hub.GetBaggage(); !strings.Contains(baggage, "sentry-release=test-release") {
		require.Failf(t, "test failure",

			"baggage = %q, want continued Sentry baggage", baggage)
	}
}

func TestUnarySentryInterceptorAddsBreadcrumb(t *testing.T) {
	t.Parallel()

	interceptor := unarySentryInterceptor(grpcSentryMetadata{})
	info := &grpc.UnaryServerInfo{FullMethod: "/strait.worker.v1.WorkerService/Heartbeat"}
	ctx := sentry.SetHubOnContext(context.Background(), sentry.NewHub(nil, sentry.NewScope()))
	_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.Unavailable, "worker unavailable")
	})
	require.Error(
		t, err)

	event := sentry.GetHubFromContext(ctx).Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	require.False(
		t, event ==
			nil || len(
			event.Breadcrumbs) != 1)

	bc := event.Breadcrumbs[0]
	require.Equal(
		t, "grpc.server",
		bc.Category,
	)
	require.Equal(
		t, codes.Unavailable.
			String(), bc.Data["grpc_code"])
	require.Equal(
		t, "Heartbeat",
		bc.Data["rpc"])

}

func TestShouldCaptureGRPCSentryError_OnlyServerSideCodes(t *testing.T) {
	t.Parallel()
	require.True(t,
		shouldCaptureGRPCSentryError(status.Error(codes.Internal,
			"boom")))
	require.False(
		t, shouldCaptureGRPCSentryError(status.Error(codes.InvalidArgument,
			"bad request")))
	require.False(
		t, shouldCaptureGRPCSentryError(status.Error(codes.Unauthenticated,
			"missing auth")))

}
