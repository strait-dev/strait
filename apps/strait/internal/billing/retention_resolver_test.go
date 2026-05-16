package billing

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
)

func TestGetOrgRetentionDays_EmptyOrgID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty org ID")
	}
	if days != 0 {
		t.Errorf("days = %d, want 0 on resolver error", days)
	}
}

func TestGetOrgRetentionDays_SubscriptionNotFound(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-no-sub")
	if !errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("error = %v, want ErrSubscriptionNotFound", err)
	}
	if days != 0 {
		t.Errorf("days = %d, want 0 on missing subscription", days)
	}
}

func TestGetOrgRetentionDays_StoreErrorDoesNotFallbackToShorterRetention(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("database unavailable")
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-pro")
	if err == nil {
		t.Fatal("expected resolver error")
	}
	if days != 0 {
		t.Errorf("days = %d, want 0 so scheduler skips retention on uncertainty", days)
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
