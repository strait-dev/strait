package billing

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		err)
	assert.Equal(t, "free",
		impact.
			TargetTier,
	)

	freeLimits := GetPlanLimits(domain.PlanFree)

	// With 5 projects and free limit of 1, projects should require reduction.
	impactMap := make(map[string]ResourceImpact)
	for _, imp := range impact.Impacts {
		impactMap[imp.Resource] = imp
	}

	projImpact := impactMap["projects"]
	assert.Equal(t, ResourceActionReduce,

		projImpact.Action)
	assert.EqualValues(t, 5,
		projImpact.
			Current,
	)
	assert.Equal(t, int64(freeLimits.
		MaxProjectsPerOrg,
	), projImpact.
		Limit,
	)
}

func TestPreviewDowngrade_SubscriptionNotFound(t *testing.T) {
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{},
	}

	_, err := PreviewDowngrade(context.Background(), store, "org-missing", domain.PlanFree)
	require.Error(t,
		err)
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
	require.NoError(t,
		err)
	require.NotEmpty(
		t, impact.
			EffectiveDate,
	)
	assert.Equal(t, "2026-04-15",

		impact.
			EffectiveDate)
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
	require.NoError(t,
		err)
	require.NotEmpty(
		t, impact.
			EffectiveDate,
	)

	// Should be end of current month.
	now := time.Now().UTC()
	endOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	expected := endOfMonth.Format("2006-01-02")
	assert.Equal(t, expected,
		impact.
			EffectiveDate,
	)
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
	require.NoError(t,
		err)

	for _, imp := range impact.Impacts {
		require.NotEqual(
			t, "regions",
			imp.Resource,
		)
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
	require.NoError(t,
		err)

	var runsImpact *ResourceImpact
	for i := range impact.Impacts {
		if impact.Impacts[i].Resource == "runs_per_month" {
			runsImpact = &impact.Impacts[i]
			break
		}
	}
	require.NotNil(t,
		runsImpact,
	)
	require.EqualValues(t, 7_000, runsImpact.
		Current,
	)
	require.Equal(t,
		int64(GetPlanLimits(
			domain.PlanFree).MaxRunsPerMonth,
		), runsImpact.Limit)
	require.Equal(t,
		ResourceActionReduce,

		runsImpact.Action)
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
	require.NoError(t,
		err)

	for _, imp := range impact.Impacts {
		assert.NotEqual(t,
			"http_mode_jobs",

			imp.Resource)
	}
}

func TestAutoDisable_LogDrains_OverLimit_Disabled(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "log_drains", Current: 5, Limit: 2, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, manual)
	assert.False(t, len(auto) !=
		1 || auto[0].Resource != "log_drains",
	)
}

func TestAutoDisable_AlertRules_OverLimit_Disabled(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "alert_rules", Current: 10, Limit: 3, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, manual)
	assert.False(t, len(auto) !=
		1 || auto[0].Resource != "alert_rules",
	)
}

func TestAutoDisable_Webhooks_OverLimit_Disabled(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "webhooks", Current: 8, Limit: 2, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, manual)
	assert.False(t, len(auto) !=
		1 || auto[0].Resource != "webhooks",
	)
}

func TestAutoDisable_BelowLimit_NoAction(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "log_drains", Current: 1, Limit: 5, Action: ResourceActionOK},
		{Resource: "alert_rules", Current: 2, Limit: 10, Action: ResourceActionOK},
		{Resource: "webhooks", Current: 0, Limit: 3, Action: ResourceActionOK},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, manual)
	assert.Empty(t, auto)
}

func TestRequiresManualAction_Projects_OverLimit_Flagged(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "projects", Current: 10, Limit: 2, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, auto)
	assert.False(t, len(manual) !=
		1 ||
		manual[0].Resource != "projects",
	)
}

func TestRequiresManualAction_Members_OverLimit_Flagged(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "members", Current: 20, Limit: 5, Action: ResourceActionReduce},
		{Resource: "members_per_org", Current: 20, Limit: 5, Action: ResourceActionReduce},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, auto)
	assert.Len(t, manual,
		2)
}

func TestRequiresManualAction_BelowLimit_Empty(t *testing.T) {
	impacts := []ResourceImpact{
		{Resource: "projects", Current: 1, Limit: 5, Action: ResourceActionOK},
		{Resource: "members", Current: 2, Limit: 10, Action: ResourceActionOK},
	}
	manual, auto := AutoDisableResources(impacts)
	assert.Empty(t, manual)
	assert.Empty(t, auto)
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
	assert.Len(t, manual,
		3)

	// projects, members, members_per_org should be manual

	for _, m := range manual {
		assert.False(t, m.
			Resource !=
			"projects" &&
			m.Resource != "members" &&
			m.Resource != "members_per_org",
		)
	}
	assert.Len(t, auto,
		4)

	// log_drains, alert_rules, webhooks, custom_roles should be auto-disabled

	for _, a := range auto {
		assert.False(t, a.
			Resource ==
			"projects" ||
			a.Resource == "members" ||
			a.Resource == "members_per_org",
		)
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
			assert.Equal(t, tt.
				expected,
				impact.
					Action,
			)
		})
	}
}
