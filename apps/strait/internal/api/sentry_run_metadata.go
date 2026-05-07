package api

import (
	"context"

	"strait/internal/domain"
)

func sentryRunMetadata(ctx context.Context, route string, metadata map[string]string) map[string]string {
	if metadata == nil {
		metadata = make(map[string]string, 3)
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
	return metadata
}
