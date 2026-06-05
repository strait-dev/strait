//go:build integration

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegrationSentryBreadcrumbChainStaysOnContextHub(t *testing.T) {
	t.Parallel()

	ctx, hub := contextWithSentryHub()
	AddSentryBreadcrumb(ctx, "queue.claim", "run claimed", map[string]any{"run_id": "run-1"})
	AddSentryBreadcrumb(ctx, "worker.dispatch", "run dispatch starting", map[string]any{"job_id": "job-1"})
	AddSentryBreadcrumb(ctx, "worker.dispatch", "worker panic", map[string]any{"error_class": "server"})

	breadcrumbs := sentryBreadcrumbsFromHub(t, hub)
	require.Len(t, breadcrumbs,

		3)
	require.False(t, breadcrumbs[0].Category !=
		"queue.claim" || breadcrumbs[1].Category != "worker.dispatch" || breadcrumbs[2].Category !=
		"worker.dispatch")

}
