package billing

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

type mockDowngradeStore struct {
	mockBillingStore
}

func TestPreviewDowngrade_ProToFree(t *testing.T) {
	now := time.Now()
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {
					OrgID:    "org-1",
					PlanTier: "pro",
					Status:   "active",
				},
			},
			projects: map[string][]string{
				"org-1": {"proj-1", "proj-2", "proj-3", "proj-4", "proj-5"},
			},
		},
	}
	_ = now

	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if impact.TargetTier != "free" {
		t.Errorf("expected target tier free, got %s", impact.TargetTier)
	}

	// With 5 projects and free limit of 2, projects should require reduction.
	impactMap := make(map[string]ResourceImpact)
	for _, imp := range impact.Impacts {
		impactMap[imp.Resource] = imp
	}

	projImpact := impactMap["projects"]
	if projImpact.Action != ResourceActionReduce {
		t.Errorf("expected projects action reduce, got %s", projImpact.Action)
	}
	if projImpact.Current != 5 {
		t.Errorf("expected current projects 5, got %d", projImpact.Current)
	}
	if projImpact.Limit != 2 {
		t.Errorf("expected limit 2 for free plan, got %d", projImpact.Limit)
	}
}

func TestPreviewDowngrade_SubscriptionNotFound(t *testing.T) {
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{},
	}

	_, err := PreviewDowngrade(context.Background(), store, "org-missing", domain.PlanFree)
	if err == nil {
		t.Fatal("expected error for missing subscription")
	}
}

func TestBuildImpact(t *testing.T) {
	tests := []struct {
		name     string
		current  int64
		limit    int64
		expected ResourceAction
	}{
		{"within_limit", 3, 5, ResourceActionOK},
		{"at_limit", 5, 5, ResourceActionOK},
		{"over_limit", 10, 5, ResourceActionReduce},
		{"removed", 5, 0, ResourceActionRemove},
		{"unlimited_to_limited", -1, 5, ResourceActionReduce},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impact := buildImpact("test", tt.current, tt.limit)
			if impact.Action != tt.expected {
				t.Errorf("buildImpact(%d, %d) action = %s, want %s", tt.current, tt.limit, impact.Action, tt.expected)
			}
		})
	}
}
