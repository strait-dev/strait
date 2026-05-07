package api

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"

	"strait/internal/domain"
)

func sentryRunMetadata(ctx context.Context, route string, metadata map[string]string) map[string]string {
	if metadata == nil {
		metadata = make(map[string]string, 7)
	}
	if actorType := actorTypeFromContext(ctx); actorType != "" {
		metadata[domain.RunMetadataSentryActorType] = actorType
	}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		metadata[domain.RunMetadataSentryRequestID] = requestID
	}
	if route != "" {
		metadata[domain.RunMetadataSentryRoute] = route
	}
	applyTraceContextMetadata(ctx, metadata)
	return metadata
}

func applyRunTraceHeaderMetadata(metadata map[string]string, traceparent, tracestate, sentryTrace, baggage string) map[string]string {
	if metadata == nil {
		metadata = make(map[string]string, 4)
	}
	if traceparent != "" {
		metadata[domain.RunMetadataTraceParent] = traceparent
		if tracestate != "" {
			metadata[domain.RunMetadataTraceState] = tracestate
		}
	}
	if sentryTrace != "" {
		metadata[domain.RunMetadataSentryTrace] = sentryTrace
		if baggage != "" {
			metadata[domain.RunMetadataSentryBaggage] = baggage
		}
	}
	return metadata
}

func applyTraceContextMetadata(ctx context.Context, metadata map[string]string) {
	if metadata == nil {
		return
	}
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		if traceparent := hub.GetTraceparent(); traceparent != "" {
			metadata[domain.RunMetadataSentryTrace] = traceparent
		}
		if baggage := hub.GetBaggage(); baggage != "" {
			metadata[domain.RunMetadataSentryBaggage] = baggage
		}
	}
	if _, ok := metadata[domain.RunMetadataTraceParent]; ok {
		return
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() || !sc.TraceID().IsValid() || !sc.SpanID().IsValid() {
		return
	}
	flags := "00"
	if sc.TraceFlags().IsSampled() {
		flags = "01"
	}
	metadata[domain.RunMetadataTraceParent] = fmt.Sprintf("00-%s-%s-%s", sc.TraceID().String(), sc.SpanID().String(), flags)
}
