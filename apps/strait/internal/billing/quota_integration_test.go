//go:build integration

package billing_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestCheckProjectLimit_Integration exercises the project-count gate against
// real Postgres so the LimitError shape, the cache invalidation path on plan
// changes, and the unlimited tier behaviour are all covered end-to-end.
func TestCheckProjectLimit_Integration(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)
	enforcer := billing.NewEnforcer(pgStore, nil, slog.Default())

	orgID := "org-quota-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure subscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanStarter), "active"); err != nil {
		t.Fatalf("upgrade to starter: %v", err)
	}
	enforcer.InvalidateOrgCache(orgID)

	// Under the Starter cap (3 projects): two creates succeed, third pushes
	// us up to the limit but the check runs *before* insertion so it should
	// still allow.
	for i := range billing.MaxProjectsStarter {
		if err := enforcer.CheckProjectLimit(ctx, orgID); err != nil {
			t.Fatalf("CheckProjectLimit under cap (i=%d) returned %v, want nil", i, err)
		}
		createProject(t, ctx, q, orgID, "p"+newID())
	}

	// We now have MaxProjectsStarter projects; a fourth must be rejected
	// with a structured LimitError that carries the canonical fields.
	err := enforcer.CheckProjectLimit(ctx, orgID)
	if err == nil {
		t.Fatal("CheckProjectLimit at cap returned nil, want *LimitError")
	}
	var le *billing.LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *billing.LimitError, got %T: %v", err, err)
	}
	if le.Code != "project_limit_reached" {
		t.Errorf("Code = %q, want project_limit_reached", le.Code)
	}
	if le.Limit != int64(billing.MaxProjectsStarter) {
		t.Errorf("Limit = %d, want %d", le.Limit, billing.MaxProjectsStarter)
	}
	if le.CurrentUsage != int64(billing.MaxProjectsStarter) {
		t.Errorf("CurrentUsage = %d, want %d", le.CurrentUsage, billing.MaxProjectsStarter)
	}
	if le.Plan != string(domain.PlanStarter) {
		t.Errorf("Plan = %q, want %q", le.Plan, domain.PlanStarter)
	}
	if le.UpgradeURL == "" {
		t.Error("UpgradeURL is empty, want non-empty fallback")
	}

	// Upgrade to Pro, invalidate the cached limits, and confirm the same
	// org can now create more projects (10 > 3 existing).
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanPro), "active"); err != nil {
		t.Fatalf("upgrade to pro: %v", err)
	}
	enforcer.InvalidateOrgCache(orgID)

	if err := enforcer.CheckProjectLimit(ctx, orgID); err != nil {
		t.Fatalf("CheckProjectLimit after upgrade returned %v, want nil", err)
	}
}

// TestCheckProjectLimit_UnlimitedTier confirms unlimited tiers (Enterprise,
// represented as MaxProjectsPerOrg = -1) short-circuit the count query so
// extremely large orgs don't pay the lookup cost on every dispatch.
func TestCheckProjectLimit_UnlimitedTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)
	enforcer := billing.NewEnforcer(pgStore, nil, slog.Default())

	orgID := "org-unlimited-" + newID()
	if err := pgStore.EnsureOrgSubscription(ctx, orgID); err != nil {
		t.Fatalf("ensure subscription: %v", err)
	}
	if err := pgStore.UpdateOrgSubscriptionPlan(ctx, orgID, string(domain.PlanEnterprise), "active"); err != nil {
		t.Fatalf("upgrade to enterprise: %v", err)
	}
	enforcer.InvalidateOrgCache(orgID)

	// Seed well beyond any finite tier and confirm we still pass.
	for range 25 {
		createProject(t, ctx, q, orgID, "p"+newID())
	}
	if err := enforcer.CheckProjectLimit(ctx, orgID); err != nil {
		t.Fatalf("CheckProjectLimit on unlimited tier returned %v, want nil", err)
	}
}
