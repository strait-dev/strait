package billing

import (
	"context"
	"errors"
	"strings"
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

func TestDeepSecGetOrgRetentionDays_AddsActiveHistoryAddons(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-scale": {
				OrgID:    "org-scale",
				PlanTier: "scale",
				Status:   "active",
			},
		},
		activeAddons: []Addon{
			{OrgID: "org-scale", AddonType: AddonHistory30d, Quantity: 2, Active: true},
			{OrgID: "org-scale", AddonType: AddonHistory30d, Quantity: 10, Active: false},
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-scale")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := GetPlanLimits(domain.PlanScale).RetentionDays + 60
	if days != want {
		t.Errorf("days = %d, want %d", days, want)
	}
}

func TestDeepSecGetOrgRetentionDays_UnlimitedRetentionRemainsUnlimited(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-enterprise": {
				OrgID:    "org-enterprise",
				PlanTier: "enterprise",
				Status:   "active",
			},
		},
		activeAddons: []Addon{
			{OrgID: "org-enterprise", AddonType: AddonHistory30d, Quantity: 10, Active: true},
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-enterprise")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != -1 {
		t.Errorf("days = %d, want -1 unlimited", days)
	}
}

func TestDeepSecGetOrgRetentionDays_AddonLookupErrorSkipsRetention(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pro": {OrgID: "org-pro", PlanTier: "pro", Status: "active"},
		},
		listActiveAddonsErr: errors.New("addon store down"),
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-pro")
	if err == nil {
		t.Fatal("expected add-on lookup error")
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

func TestListAllSubscribedOrgIDsSQL_HasNoFixedRetentionSweepCap(t *testing.T) {
	t.Parallel()

	sql := strings.ToUpper(listAllSubscribedOrgIDsSQL())
	if strings.Contains(sql, "LIMIT") {
		t.Fatalf("subscribed org listing must not use a fixed LIMIT: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY ORG_ID") {
		t.Fatalf("subscribed org listing should remain deterministic: %s", sql)
	}
}
