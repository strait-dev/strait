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

	freeLimits := GetPlanLimits(domain.PlanFree)

	// With 5 projects and free limit of 1, projects should require reduction.
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
	if projImpact.Limit != int64(freeLimits.MaxProjectsPerOrg) {
		t.Errorf("expected limit %d for free plan, got %d", freeLimits.MaxProjectsPerOrg, projImpact.Limit)
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

func TestPreviewDowngrade_IncludesEffectiveDate(t *testing.T) {
	periodEnd := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {
					OrgID:            "org-1",
					PlanTier:         "pro",
					Status:           "active",
					CurrentPeriodEnd: &periodEnd,
				},
			},
			projects: map[string][]string{
				"org-1": {"proj-1"},
			},
		},
	}

	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if impact.EffectiveDate == "" {
		t.Fatal("expected non-empty effective date")
	}
	if impact.EffectiveDate != "2026-04-15" {
		t.Errorf("expected effective date 2026-04-15, got %s", impact.EffectiveDate)
	}
}

func TestPreviewDowngrade_EffectiveDate_NilPeriod_DefaultsToEndOfMonth(t *testing.T) {
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {
					OrgID:    "org-1",
					PlanTier: "pro",
					Status:   "active",
					// CurrentPeriodEnd is nil
				},
			},
			projects: map[string][]string{
				"org-1": {"proj-1"},
			},
		},
	}

	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if impact.EffectiveDate == "" {
		t.Fatal("expected non-empty effective date when period end is nil")
	}

	// Should be end of current month.
	now := time.Now().UTC()
	endOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	expected := endOfMonth.Format("2006-01-02")
	if impact.EffectiveDate != expected {
		t.Errorf("expected effective date %s (end of month), got %s", expected, impact.EffectiveDate)
	}
}

func TestPreviewDowngrade_DoesNotExposeLaunchInactiveRegions(t *testing.T) {
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
			projects: map[string][]string{
				"org-1": {"proj-1"},
			},
		},
	}
	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, imp := range impact.Impacts {
		if imp.Resource == "regions" {
			t.Fatalf("downgrade preview exposed launch-inactive regions impact: %#v", imp)
		}
	}
}

func TestPreviewDowngrade_UsesActualPeriodRunsForMonthlyImpact(t *testing.T) {
	periodStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {
					OrgID:              "org-1",
					PlanTier:           string(domain.PlanPro),
					Status:             "active",
					CurrentPeriodStart: &periodStart,
					CurrentPeriodEnd:   &periodEnd,
				},
			},
			projects: map[string][]string{
				"org-1": {"proj-1"},
			},
			usageRecords: []UsageRecord{
				{OrgID: "org-1", ProjectID: "proj-1", PeriodDate: periodStart, RunsCount: 4_000},
				{OrgID: "org-1", ProjectID: "proj-1", PeriodDate: periodStart.AddDate(0, 0, 1), RunsCount: 3_000},
			},
		},
	}

	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var runsImpact *ResourceImpact
	for i := range impact.Impacts {
		if impact.Impacts[i].Resource == "runs_per_month" {
			runsImpact = &impact.Impacts[i]
			break
		}
	}
	if runsImpact == nil {
		t.Fatal("expected runs_per_month impact")
		return
	}
	if runsImpact.Current != 7_000 {
		t.Fatalf("runs_per_month current = %d, want actual period usage 7000", runsImpact.Current)
	}
	if runsImpact.Limit != int64(GetPlanLimits(domain.PlanFree).MaxRunsPerMonth) {
		t.Fatalf("runs_per_month limit = %d, want free monthly cap", runsImpact.Limit)
	}
	if runsImpact.Action != ResourceActionReduce {
		t.Fatalf("runs_per_month action = %s, want reduce", runsImpact.Action)
	}
}

func TestPreviewDowngrade_HTTPJobsImpact(t *testing.T) {
	// HTTP mode is available on all tiers; downgrading between any two tiers
	// does not remove HTTP mode access. The http_mode_jobs impact is therefore
	// never emitted regardless of the source/target tier combination.
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
			projects: map[string][]string{
				"org-1": {"proj-1"},
			},
			httpJobCount: 3,
		},
	}
	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, imp := range impact.Impacts {
		if imp.Resource == "http_mode_jobs" {
			t.Error("unexpected http_mode_jobs impact: HTTP mode is available on all tiers")
		}
	}
}

func TestAutoDisable_LogDrains_OverLimit_Disabled(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "log_drains", Current: 5, Limit: 2, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(manual) != 0 {
		t.Errorf("expected no manual actions, got %d", len(manual))
	}
	if len(auto) != 1 || auto[0].Resource != "log_drains" {
		t.Errorf("expected log_drains in auto-disabled, got %v", auto)
	}
}

func TestAutoDisable_AlertRules_OverLimit_Disabled(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "alert_rules", Current: 10, Limit: 3, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(manual) != 0 {
		t.Errorf("expected no manual actions, got %d", len(manual))
	}
	if len(auto) != 1 || auto[0].Resource != "alert_rules" {
		t.Errorf("expected alert_rules in auto-disabled, got %v", auto)
	}
}

func TestAutoDisable_Webhooks_OverLimit_Disabled(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "webhooks", Current: 8, Limit: 2, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(manual) != 0 {
		t.Errorf("expected no manual actions, got %d", len(manual))
	}
	if len(auto) != 1 || auto[0].Resource != "webhooks" {
		t.Errorf("expected webhooks in auto-disabled, got %v", auto)
	}
}

func TestAutoDisable_BelowLimit_NoAction(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "log_drains", Current: 1, Limit: 5, Action: ResourceActionOK},
		{Resource: "alert_rules", Current: 2, Limit: 10, Action: ResourceActionOK},
		{Resource: "webhooks", Current: 0, Limit: 3, Action: ResourceActionOK},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(manual) != 0 {
		t.Errorf("expected no manual actions for below-limit resources, got %d", len(manual))
	}
	if len(auto) != 0 {
		t.Errorf("expected no auto-disabled for below-limit resources, got %d", len(auto))
	}
}

func TestRequiresManualAction_Projects_OverLimit_Flagged(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "projects", Current: 10, Limit: 2, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(auto) != 0 {
		t.Errorf("expected no auto-disabled for projects, got %d", len(auto))
	}
	if len(manual) != 1 || manual[0].Resource != "projects" {
		t.Errorf("expected projects in manual actions, got %v", manual)
	}
}

func TestRequiresManualAction_Members_OverLimit_Flagged(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "members", Current: 20, Limit: 5, Action: ResourceActionReduce},
		{Resource: "members_per_org", Current: 20, Limit: 5, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(auto) != 0 {
		t.Errorf("expected no auto-disabled for members, got %d", len(auto))
	}
	if len(manual) != 2 {
		t.Errorf("expected 2 manual actions for members, got %d", len(manual))
	}
}

func TestRequiresManualAction_BelowLimit_Empty(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "projects", Current: 1, Limit: 5, Action: ResourceActionOK},
		{Resource: "members", Current: 2, Limit: 10, Action: ResourceActionOK},
	}
	manual, auto := AutoDisableResources(impacts)
	if len(manual) != 0 {
		t.Errorf("expected no manual actions for below-limit, got %d", len(manual))
	}
	if len(auto) != 0 {
		t.Errorf("expected no auto-disabled for below-limit, got %d", len(auto))
	}
}

func TestAutoDisable_OnlyNonCritical(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "projects", Current: 10, Limit: 2, Action: ResourceActionReduce},
		{Resource: "members", Current: 20, Limit: 5, Action: ResourceActionReduce},
		{Resource: "members_per_org", Current: 20, Limit: 5, Action: ResourceActionReduce},
		{Resource: "log_drains", Current: 5, Limit: 1, Action: ResourceActionReduce},
		{Resource: "alert_rules", Current: 8, Limit: 2, Action: ResourceActionReduce},
		{Resource: "webhooks", Current: 6, Limit: 1, Action: ResourceActionReduce},
		{Resource: "custom_roles", Current: 3, Limit: 0, Action: ResourceActionRemove},
	}
	manual, auto := AutoDisableResources(impacts)

	// projects, members, members_per_org should be manual
	if len(manual) != 3 {
		t.Errorf("expected 3 manual actions, got %d", len(manual))
	}
	for _, m := range manual {
		if m.Resource != "projects" && m.Resource != "members" && m.Resource != "members_per_org" {
			t.Errorf("unexpected resource in manual actions: %s", m.Resource)
		}
	}

	// log_drains, alert_rules, webhooks, custom_roles should be auto-disabled
	if len(auto) != 4 {
		t.Errorf("expected 4 auto-disabled, got %d", len(auto))
	}
	for _, a := range auto {
		if a.Resource == "projects" || a.Resource == "members" || a.Resource == "members_per_org" {
			t.Errorf("critical resource %s should not be in auto-disabled", a.Resource)
		}
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
