//go:build integration

package telemetry

import (
	"errors"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
)

func TestIntegrationSentryFingerprintAndRelease(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{
		Tags: map[string]string{
			"subsystem":   SubsystemWorkflow,
			"error_class": "server",
		},
		Breadcrumbs: []*sentry.Breadcrumb{{
			Category: "workflow.step",
			Data: map[string]any{
				"step_type": "job",
			},
		}},
	}
	got := BeforeSend(event, &sentry.EventHint{OriginalException: errors.New("workflow step failed")})
	require.NotNil(t, got)

	require.Equal(t, []string{"workflow", "job", "server"}, got.Fingerprint)
	require.Equal(t, "v1.2.3+abcdef123456",

		BuildSentryRelease("v1.2.3",

			"abcdef1234567890"))

}
