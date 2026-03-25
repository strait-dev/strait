package api

import (
	"context"
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

func TestCheckHTTPModeAllowed_FreePlanRejected(t *testing.T) {
	t.Parallel()

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
	if err == nil {
		t.Fatal("expected error for free plan HTTP mode")
	}
}

func TestCheckHTTPModeAllowed_StarterPlanRejected(t *testing.T) {
	t.Parallel()

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
	if err == nil {
		t.Fatal("expected error for starter plan HTTP mode")
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

func TestCheckHTTPModeAllowed_ManagedModeSkipped(t *testing.T) {
	t.Parallel()

	// Managed mode should always pass (not gated).
	s := &Server{
		edition: domain.EditionCloud,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeManaged, "proj-1")
	if err != nil {
		t.Fatalf("expected no error for managed mode, got: %v", err)
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
