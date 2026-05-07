package grpc

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
		mode:    "all",
		region:  "iad",
		version: "test-version",
	}, map[telemetry.SentryTag]string{
		telemetry.TagService: "strait.worker.v1.WorkerService",
		telemetry.TagRPC:     "StreamTasks",
	})
	configureGRPCSentryWorkerScope(ctx, "worker-1", "Worker One", "host-a", "go", "1.2.3")

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil {
		t.Fatal("expected event")
	}

	wantTags := map[string]string{
		"project_id":     "proj-1",
		"org_id":         "org-1",
		"api_key_id":     "key-1",
		"actor_id":       "apikey:key-1",
		"actor_type":     "api_key",
		"environment_id": "env-1",
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
		if got := event.Tags[key]; got != want {
			t.Fatalf("tag %s = %q, want %q", key, got, want)
		}
	}
	if event.User.ID != "apikey:key-1" {
		t.Fatalf("user id = %q, want apikey:key-1", event.User.ID)
	}
	if event.Contexts["grpc.request"]["project_id"] != "proj-1" {
		t.Fatalf("grpc.request project_id = %v, want proj-1", event.Contexts["grpc.request"]["project_id"])
	}
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
	if err == nil {
		t.Fatal("expected handler error")
	}
	if !sawHub {
		t.Fatal("expected wrapped stream context to contain Sentry hub")
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
	if err == nil {
		t.Fatal("expected handler error")
	}

	event := sentry.GetHubFromContext(ctx).Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil || len(event.Breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs = %v, want one breadcrumb", event)
	}
	bc := event.Breadcrumbs[0]
	if bc.Category != "grpc.server" {
		t.Fatalf("category = %q, want grpc.server", bc.Category)
	}
	if got := bc.Data["grpc_code"]; got != codes.Unavailable.String() {
		t.Fatalf("grpc_code = %v, want %s", got, codes.Unavailable.String())
	}
	if got := bc.Data["rpc"]; got != "Heartbeat" {
		t.Fatalf("rpc = %v, want Heartbeat", got)
	}
}

func TestShouldCaptureGRPCSentryError_OnlyServerSideCodes(t *testing.T) {
	t.Parallel()

	if !shouldCaptureGRPCSentryError(status.Error(codes.Internal, "boom")) {
		t.Fatal("expected Internal to be captured")
	}
	if shouldCaptureGRPCSentryError(status.Error(codes.InvalidArgument, "bad request")) {
		t.Fatal("expected InvalidArgument to be ignored")
	}
	if shouldCaptureGRPCSentryError(status.Error(codes.Unauthenticated, "missing auth")) {
		t.Fatal("expected Unauthenticated to be ignored")
	}
}
