package api

import (
	"context"
	"testing"

	"strait/internal/domain"
)

// TestGetProjectPlanTierCtx_ReadsFromBillingEnforcer verifies that
// getProjectPlanTierCtx reads plan tier from the billing enforcer
// (org_subscriptions) rather than the dead project_quotas.plan_tier column.
func TestGetProjectPlanTierCtx_ReadsFromBillingEnforcer(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{
		jobsPlanByProject: map[string]domain.PlanTier{
			"proj-paid": domain.PlanPro,
			"proj-free": domain.PlanFree,
		},
	}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})

	cases := []struct {
		projectID string
		want      domain.PlanTier
	}{
		{"proj-paid", domain.PlanPro},
		{"proj-free", domain.PlanFree},
		{"proj-unknown", domain.PlanFree}, // fail-open default
		{"", domain.PlanFree},
	}
	for _, tc := range cases {
		got := srv.getProjectPlanTierCtx(context.Background(), tc.projectID)
		if got != tc.want {
			t.Errorf("getProjectPlanTierCtx(%q) = %q, want %q", tc.projectID, got, tc.want)
		}
	}
}

// TestGetProjectPlanTierCtx_NilEnforcerReturnsFree guards against a nil
// billing enforcer, which is possible in tests that instantiate Server
// without billing wired up.
func TestGetProjectPlanTierCtx_NilEnforcerReturnsFree(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	if tier := srv.getProjectPlanTierCtx(context.Background(), "anything"); tier != domain.PlanFree {
		t.Errorf("expected PlanFree with nil enforcer, got %q", tier)
	}
}
