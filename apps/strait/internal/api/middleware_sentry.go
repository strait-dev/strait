package api

import (
	"context"
	"fmt"
	"maps"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

// sentryScope ensures every request has an isolated hub and a baseline scope.
// Authentication middleware calls serveWithSentryScope again after it stamps
// actor/project context, so events captured inside handlers include both the
// transport metadata and the resolved caller.
func (s *Server) sentryScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub := sentry.GetHubFromContext(r.Context())
		if hub == nil {
			hub = sentry.CurrentHub().Clone()
			r = r.WithContext(sentry.SetHubOnContext(r.Context(), hub))
		}
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
		applyHTTPSentryScope(scope, r, s.edition)
		scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			applyHTTPSentryEvent(event, r, s.edition)
			return event
		})
	})
}

func applyHTTPSentryScope(scope *sentry.Scope, r *http.Request, edition any) {
	scope.SetRequest(r)
	tags := sentryHTTPContextTags(r, edition)
	for k, v := range tags {
		scope.SetTag(k, v)
	}
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

func applyHTTPSentryEvent(event *sentry.Event, r *http.Request, edition any) {
	if event.Tags == nil {
		event.Tags = map[string]string{}
	}
	maps.Copy(event.Tags, sentryHTTPContextTags(r, edition))
	if actorID := actorFromContext(r.Context()); actorID != "" && event.User.ID == "" {
		event.User = sentry.User{
			ID: actorID,
			Data: map[string]string{
				"actor_type": actorTypeFromContext(r.Context()),
				"project_id": projectIDFromContext(r.Context()),
			},
		}
	}
	if event.Contexts == nil {
		event.Contexts = map[string]sentry.Context{}
	}
	event.Contexts["http.request"] = sentry.Context{
		"method":     r.Method,
		"path":       r.URL.Path,
		"route":      chiRoutePattern(r),
		"request_id": requestIDFromContext(r.Context()),
	}
}

func sentryHTTPContextTags(r *http.Request, edition any) map[string]string {
	ctx := r.Context()
	tags := map[string]string{
		"http_method": r.Method,
		"edition":     stringValue(edition),
	}
	if route := chiRoutePattern(r); route != "" {
		tags["http_route"] = route
	} else if r.URL != nil {
		tags["http_path"] = r.URL.Path
	}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		tags["request_id"] = requestID
	}
	if projectID := projectIDFromContext(ctx); projectID != "" {
		tags["project_id"] = projectID
	}
	if actorID := actorFromContext(ctx); actorID != "" {
		tags["actor_id"] = actorID
	}
	if actorType := actorTypeFromContext(ctx); actorType != "" {
		tags["actor_type"] = actorType
	}
	if traceID, spanID := otelTraceIDs(ctx); traceID != "" {
		tags["trace_id"] = traceID
		if spanID != "" {
			tags["span_id"] = spanID
		}
	}
	if runID, _ := ctx.Value(ctxRunIDKey).(string); runID != "" {
		tags["run_id"] = runID
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
