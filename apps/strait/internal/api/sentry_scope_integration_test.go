//go:build integration

package api

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestIntegrationSentryRunMetadataCarriesRequestScope(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxRequestIDKey, "req-integration")

	got := sentryRunMetadata(ctx, "POST /v1/jobs/{jobID}/trigger", map[string]string{
		"dependency_key": "dep-1",
	})

	if got["dependency_key"] != "dep-1" {
		t.Fatalf("dependency_key = %q, want dep-1", got["dependency_key"])
	}
	if got[domain.RunMetadataSentryActorType] != "api_key" {
		t.Fatalf("actor type metadata = %q, want api_key", got[domain.RunMetadataSentryActorType])
	}
	if got[domain.RunMetadataSentryRequestID] != "req-integration" {
		t.Fatalf("request id metadata = %q, want req-integration", got[domain.RunMetadataSentryRequestID])
	}
	if got[domain.RunMetadataSentryRoute] != "POST /v1/jobs/{jobID}/trigger" {
		t.Fatalf("route metadata = %q, want trigger route", got[domain.RunMetadataSentryRoute])
	}
}
