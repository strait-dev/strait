package billing

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"

	"strait/internal/telemetry"
)

func TestBillingSentryBreadcrumbUsesContextHub(t *testing.T) {
	t.Parallel()

	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	addBillingSentryBreadcrumb(ctx, "plan_limits", "billing plan limits requested", map[string]any{
		"org_id": "org-123",
	})
	enforcer := &Enforcer{sentryMode: "all", sentryRegion: "iad", sentryVersion: "test-version"}
	hub.ConfigureScope(func(scope *sentry.Scope) {
		enforcer.applyBillingSentryScope(scope, "org-123", "plan_limits")
	})

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t,
		event)
	require.Equal(t,
		telemetry.SubsystemBilling,

		event.Tags[string(
			telemetry.
				TagSubsystem)])
	require.Equal(t,
		"org-123",
		event.
			Tags[string(telemetry.TagOrgID)])
	require.Equal(t,
		"all", event.
			Tags[string(telemetry.TagMode)])
	require.Equal(t,
		"iad", event.
			Tags[string(telemetry.TagRegion)],
	)
	require.Equal(t,
		"test-version",

		event.Tags[string(telemetry.TagVersion)])
	require.Len(t, event.
		Breadcrumbs,

		1)
	require.Equal(t,
		"billing.plan_limits",

		event.Breadcrumbs[0].Category,
	)
}
