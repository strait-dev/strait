package billing

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestGetOrgRetentionDays_EmptyOrgID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GetPlanLimits(domain.PlanFree).RetentionDays
	if days != want {
		t.Errorf("days = %d, want %d (free tier)", days, want)
	}
}

func TestGetOrgRetentionDays_SubscriptionNotFound(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-no-sub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GetPlanLimits(domain.PlanFree).RetentionDays
	if days != want {
		t.Errorf("days = %d, want %d (free tier fallback)", days, want)
	}
}

func TestGetOrgRetentionDays_ProPlan(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pro": {OrgID: "org-pro", PlanTier: "pro", Status: "active"},
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-pro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GetPlanLimits(domain.PlanPro).RetentionDays
	if days != want {
		t.Errorf("days = %d, want %d (pro tier)", days, want)
	}
}

func TestListAllSubscribedOrgIDs(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	ids, err := resolver.ListAllSubscribedOrgIDs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids != nil {
		t.Errorf("expected nil, got %v", ids)
	}
}
