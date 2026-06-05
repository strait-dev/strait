//go:build integration

package api

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestIntegrationSentryRunMetadataCarriesRequestScope(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxRequestIDKey, "req-integration")

	got := sentryRunMetadata(ctx, "POST /v1/jobs/{jobID}/trigger", map[string]string{
		"dependency_key": "dep-1",
	})
	require.Equal(t,

		"dep-1",
		got["dependency_key"])
	require.Equal(t,

		"api_key",
		got[domain.
			RunMetadataSentryActorType])
	require.Equal(t,

		"req-integration",
		got[domain.RunMetadataSentryRequestID])
	require.Equal(t,

		"POST /v1/jobs/{jobID}/trigger",

		got[domain.RunMetadataSentryRoute])

}
