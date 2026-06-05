package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"strait/internal/domain"
)

func TestHTTPSentryScope_AttachesActorProjectRouteAndTrace(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run-1", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.RoutePatterns = []string{"/v1", "/runs/{runID}"}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxOrgIDKey, "org-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxRequestIDKey, "req-1")
	traceID := trace.TraceID{1, 2, 3}
	spanID := trace.SpanID{4, 5, 6}
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	}))
	req = req.WithContext(ctx)

	scope := sentry.NewScope()
	applyHTTPSentryScope(scope, req, sentryHTTPMetadata{
		edition: string(domain.EditionCloud),
		mode:    "all",
		region:  "iad",
		version: "test-version",
	})
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t, event)

	wantTags := map[string]string{
		"project_id":     "proj-1",
		"org_id":         "org-1",
		"environment_id": "env-1",
		"actor_id":       "user-1",
		"actor_type":     "user",
		"request_id":     "req-1",
		"method":         http.MethodPost,
		"route":          "/v1/runs/{runID}",
		"edition":        "cloud",
		"subsystem":      "api",
		"mode":           "all",
		"region":         "iad",
		"version":        "test-version",
		"trace_id":       traceID.String(),
		"span_id":        spanID.String(),
	}
	for key, want := range wantTags {
		require.Equal(t, want, event.
			Tags[key])
	}
	require.Equal(t, "user-1",
		event.User.ID)
	require.Equal(t, "/v1/runs/{runID}",
		event.Contexts["http.request"]["route"])
}

func TestHTTPSentryScope_AnonymousRequestKeepsRouteAndMethod(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.RoutePatterns = []string{"/health"}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	scope := sentry.NewScope()
	applyHTTPSentryScope(scope, req, sentryHTTPMetadata{edition: string(domain.EditionCommunity)})
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t, event)
	require.Equal(t, http.MethodGet,
		event.Tags["method"])
	require.Equal(t, "/health",
		event.Tags["route"])
	require.Empty(t, event.
		User.ID)
}

func TestSentryHTTPMiddlewareCreatesIsolatedHub(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCommunity}
	sentryHandler := sentryhttp.New(sentryhttp.Options{})
	var firstHub, secondHub *sentry.Hub
	handler := sentryHandler.Handle(srv.sentryScope(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hub := sentry.GetHubFromContext(r.Context())
		assert.NotNil(t, hub)

		if firstHub == nil {
			firstHub = hub
			return
		}
		secondHub = hub
	})))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/b", nil))
	require.False(t, firstHub ==
		nil || secondHub ==
		nil)
	require.NotEqual(t, secondHub,
		firstHub)
}
