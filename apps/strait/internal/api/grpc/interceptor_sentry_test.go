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

	interceptor := streamSentryInterceptor()
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
