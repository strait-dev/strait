package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"

	"strait/internal/telemetry"
)

// sentryScope enriches the request-specific hub created by sentryhttp.
func (s *Server) sentryScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.configureHTTPSentryScope(r)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveWithSentryScope(next http.Handler, w http.ResponseWriter, r *http.Request) {
	s.configureHTTPSentryScope(r)
	next.ServeHTTP(w, r)
}

func (s *Server) configureHTTPSentryScope(r *http.Request) {
	hub := sentry.GetHubFromContext(r.Context())
	if hub == nil {
		return
	}
	hub.ConfigureScope(func(scope *sentry.Scope) {
		applyHTTPSentryScope(scope, r, s.sentryHTTPMetadata())
	})
}

type sentryHTTPMetadata struct {
	edition string
	mode    string
	region  string
	version string
}

func (s *Server) sentryHTTPMetadata() sentryHTTPMetadata {
	meta := sentryHTTPMetadata{
		edition: stringValue(s.edition),
		version: s.version,
	}
	if s.config != nil {
		meta.mode = s.config.Mode
		meta.region = s.config.DefaultRegion
	}
	return meta
}

func applyHTTPSentryScope(scope *sentry.Scope, r *http.Request, meta sentryHTTPMetadata) {
	scope.SetRequest(r)
	telemetry.ApplySentryTags(scope, sentryHTTPContextTags(r, meta))
	if actorID := actorFromContext(r.Context()); actorID != "" {
		scope.SetUser(sentry.User{
			ID: actorID,
			Data: map[string]string{
				"actor_type": actorTypeFromContext(r.Context()),
				"project_id": projectIDFromContext(r.Context()),
			},
		})
	}
	scope.SetContext("http.request", sentry.Context{
		"method":     r.Method,
		"path":       r.URL.Path,
		"route":      chiRoutePattern(r),
		"request_id": requestIDFromContext(r.Context()),
	})
}

func sentryHTTPContextTags(r *http.Request, meta sentryHTTPMetadata) map[telemetry.SentryTag]string {
	ctx := r.Context()
	tags := telemetry.RequiredSentryTags(
		meta.edition,
		telemetry.SubsystemAPI,
		meta.mode,
		meta.region,
		meta.version,
	)
	tags[telemetry.TagMethod] = r.Method
	if route := chiRoutePattern(r); route != "" {
		tags[telemetry.TagRoute] = route
	} else if r.URL != nil {
		tags[telemetry.TagRoute] = r.URL.Path
	}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		tags[telemetry.TagRequestID] = requestID
	}
	if projectID := projectIDFromContext(ctx); projectID != "" {
		tags[telemetry.TagProjectID] = projectID
	}
	if actorID := actorFromContext(ctx); actorID != "" {
		tags[telemetry.TagActorID] = actorID
	}
	if actorType := actorTypeFromContext(ctx); actorType != "" {
		tags[telemetry.TagActorType] = actorType
	}
	if traceID, spanID := otelTraceIDs(ctx); traceID != "" {
		tags[telemetry.TagTraceID] = traceID
		if spanID != "" {
			tags[telemetry.TagSpanID] = spanID
		}
	}
	if runID, _ := ctx.Value(ctxRunIDKey).(string); runID != "" {
		tags[telemetry.TagRunID] = runID
	}
	return tags
}

func actorTypeFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxActorTypeKey).(string); ok {
		return v
	}
	return ""
}

func chiRoutePattern(r *http.Request) string {
	if r == nil {
		return ""
	}
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		return routeCtx.RoutePattern()
	}
	return ""
}

func otelTraceIDs(ctx context.Context) (string, string) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "", ""
	}
	traceID := sc.TraceID()
	if !traceID.IsValid() {
		return "", ""
	}
	spanID := sc.SpanID()
	if !spanID.IsValid() {
		return traceID.String(), ""
	}
	return traceID.String(), spanID.String()
}

func stringValue(v any) string {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return fmt.Sprint(v)
}
