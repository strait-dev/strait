package billing

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"

	"strait/internal/telemetry"
)

func TestBillingSentryBreadcrumbUsesContextHub(t *testing.T) {
	t.Parallel()

	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	addBillingSentryBreadcrumb(ctx, "plan_limits", "billing plan limits requested", map[string]any{
		"org_id": "org-123",
	})
	hub.ConfigureScope(func(scope *sentry.Scope) {
		applyBillingSentryScope(scope, "org-123", "plan_limits")
	})

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil {
		t.Fatal("expected event")
	}
	if got := event.Tags[string(telemetry.TagSubsystem)]; got != telemetry.SubsystemBilling {
		t.Fatalf("subsystem tag = %q, want %q", got, telemetry.SubsystemBilling)
	}
	if got := event.Tags[string(telemetry.TagOrgID)]; got != "org-123" {
		t.Fatalf("org_id tag = %q, want org-123", got)
	}
	if len(event.Breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs = %d, want 1", len(event.Breadcrumbs))
	}
	if got := event.Breadcrumbs[0].Category; got != "billing.plan_limits" {
		t.Fatalf("breadcrumb category = %q, want billing.plan_limits", got)
	}
}
