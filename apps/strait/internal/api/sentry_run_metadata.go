package api

import (
	"context"
	"fmt"
	"maps"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"

	"strait/internal/domain"
)

func sentryRunMetadata(ctx context.Context, route string, metadata map[string]string) map[string]string {
	metadata = copyStringMapWithCapacity(metadata, 7)
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

// Caps for trace headers persisted in run metadata. validateTriggerTraceHeaders
// rejects oversized headers up-front; truncateTraceHeader is defense-in-depth
// for any non-trigger code path that still feeds these values to the metadata
// helper.
const (
	maxTraceparentLen = 256
	maxTraceHeaderLen = 8192
)

func truncateTraceHeader(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

func validateTriggerTraceHeaders(input *TriggerJobInput) error {
	if len(input.Traceparent) > maxTraceparentLen {
		return fmt.Errorf("traceparent header exceeds maximum length of %d bytes", maxTraceparentLen)
	}
	if len(input.Tracestate) > maxTraceHeaderLen {
		return fmt.Errorf("tracestate header exceeds maximum length of %d bytes", maxTraceHeaderLen)
	}
	if len(input.SentryTrace) > maxTraceHeaderLen {
		return fmt.Errorf("sentry-trace header exceeds maximum length of %d bytes", maxTraceHeaderLen)
	}
	if len(input.Baggage) > maxTraceHeaderLen {
		return fmt.Errorf("baggage header exceeds maximum length of %d bytes", maxTraceHeaderLen)
	}
	return nil
}

func applyRunTraceHeaderMetadata(metadata map[string]string, traceparent, tracestate, sentryTrace, baggage string) map[string]string {
	metadata = copyStringMapWithCapacity(metadata, 4)
	if traceparent != "" {
		metadata[domain.RunMetadataTraceParent] = truncateTraceHeader(traceparent, maxTraceparentLen)
		if tracestate != "" {
			metadata[domain.RunMetadataTraceState] = truncateTraceHeader(tracestate, maxTraceHeaderLen)
		}
	}
	if sentryTrace != "" {
		metadata[domain.RunMetadataSentryTrace] = truncateTraceHeader(sentryTrace, maxTraceHeaderLen)
		if baggage != "" {
			metadata[domain.RunMetadataSentryBaggage] = truncateTraceHeader(baggage, maxTraceHeaderLen)
		}
	}
	return metadata
}

func copyStringMapWithCapacity(in map[string]string, extra int) map[string]string {
	out := make(map[string]string, len(in)+extra)
	maps.Copy(out, in)
	return out
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
	if !validRunTraceSpanContext(sc) {
		return
	}
	flags := "00"
	if sc.TraceFlags().IsSampled() {
		flags = "01"
	}
	metadata[domain.RunMetadataTraceParent] = fmt.Sprintf("00-%s-%s-%s", sc.TraceID().String(), sc.SpanID().String(), flags)
}

func validRunTraceSpanContext(sc trace.SpanContext) bool {
	if !sc.IsValid() {
		return false
	}
	if !sc.TraceID().IsValid() {
		return false
	}
	return sc.SpanID().IsValid()
}
