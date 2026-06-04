package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// mockHTTPModeEnforcer implements BillingEnforcer with configurable plan limits.
type mockHTTPModeEnforcer struct {
	mockBillingEnforcer
	planLimits billing.OrgPlanLimits
}

func (m *mockHTTPModeEnforcer) GetOrgPlanLimits(_ context.Context, _ string) (billing.OrgPlanLimits, error) {
	return m.planLimits, nil
}

func TestCheckHTTPModeAllowed_FreePlanAllowed(t *testing.T) {
	t.Parallel()

	// HTTP mode is available on all plans including free.
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanFree),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for free plan HTTP mode, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_StarterPlanAllowed(t *testing.T) {
	t.Parallel()

	// HTTP mode is available on all plans including starter.
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanStarter),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for starter plan HTTP mode, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_ProPlanAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanPro),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for pro plan, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_CommunityEditionAllowed(t *testing.T) {
	t.Parallel()

	// Community edition should not gate HTTP mode regardless of plan.
	s := &Server{
		edition: domain.EditionCommunity,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for community edition, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_WorkerModeSkipped(t *testing.T) {
	t.Parallel()

	// Worker mode should always pass (not gated).
	s := &Server{
		edition: domain.EditionCloud,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeWorker, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for worker mode, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_NoBillingEnforcerAllowed(t *testing.T) {
	t.Parallel()

	// No billing enforcer (e.g., dev mode) should not block.
	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: nil,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err != nil {
		t.Fatalf("expected no error with nil enforcer, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_EnterprisePlanAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanEnterprise),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for enterprise plan, got: %v", err)
	}
}

func TestCheckHTTPModeAllowed_UnavailablePlanDoesNotAdvertiseUpgrade(t *testing.T) {
	t.Parallel()

	limits := billing.GetPlanLimits(domain.PlanFree)
	limits.AllowsHTTPMode = false
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: limits,
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	if err == nil {
		t.Fatal("expected error for corrupted plan limits that disable HTTP mode")
	}
	msg := err.Error()
	for _, forbidden := range []string{"Pro plan", "$49.99", "Upgrade"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("HTTP mode fallback error advertises stale upgrade copy %q in %q", forbidden, msg)
		}
	}
}
