package billing

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUsageServiceTest(t *testing.T, store *mockBillingStore) (*UsageService, *Enforcer) {
	t.Helper()

	if store == nil {
		store = &mockBillingStore{}
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	enforcer := NewEnforcer(store, rdb, slog.Default())

	t.Cleanup(func() {
		_ = rdb.Close()
	})

	return NewUsageService(store, enforcer), enforcer
}

func assertFloatApprox(t *testing.T, got, want float64) {
	t.Helper()
	require.LessOrEqual(t, math.Abs(got-
		want), 0.0001,
	)

}

func TestUsageService_GetCurrentUsage(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		projects: map[string][]string{
			"org_test": {"proj-1"},
		},
		memberCounts: map[string]int{
			"org_test": 2,
		},
		executingRuns: map[string]int{
			"org_test": 3,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_test")
	require.NoError(t,
		err)
	assert.Equal(t, "free",
		resp.
			Plan)

	freeLimits := GetPlanLimits(domain.PlanFree)
	assert.Equal(t, int64(freeLimits.
		MaxRunsPerMonth,
	), resp.Usage.
		MonthlyRuns.
		Limit)
	assert.Equal(t, resp.
		Usage.MonthlyRuns,

		resp.Usage.
			RunsToday,
	)
	assert.EqualValues(t, 1,
		resp.Usage.
			Projects.
			Used)
	assert.EqualValues(t, 2,
		resp.Usage.
			Members.
			Used)
	assert.EqualValues(t, 3,
		resp.Usage.
			ConcurrentRuns.
			Used,
	)

	// ConcurrentRuns: 3 used / ConcurrentFree limit.
	expectedConcPct := safePercent(3, int64(freeLimits.MaxConcurrentRuns))
	assertFloatApprox(t, resp.Usage.ConcurrentRuns.Percent, expectedConcPct)
	// Members: 2 used / MaxMembersFree limit.
	expectedMemberPct := safePercent(2, int64(freeLimits.MaxMembersPerOrg))
	assertFloatApprox(t, resp.Usage.Members.Percent, expectedMemberPct)
	assert.Equal(t, RetentionFree,

		resp.Usage.
			RetentionDays,
	)

}

func TestUsageService_GetCurrentUsage_FiltersRoadmapAddons(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		activeAddons: []Addon{
			{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
			{AddonType: AddonComplianceArchive, Quantity: 1, Active: true},
			{AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true},
			{AddonType: AddonEnvironments5, Quantity: 3, Active: false},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_test")
	require.NoError(t,
		err)
	require.Len(t, resp.
		ActiveAddons,
		1)
	require.Equal(t,
		string(AddonConcurrency100), resp.
			ActiveAddons[0].
			Type,
	)
	require.EqualValues(t, 2, resp.ActiveAddons[0].Quantity)

}

func TestUsageService_NoAlertsForLowMonthlyRuns(t *testing.T) {
	t.Parallel()

	svc, enforcer := newUsageServiceTest(t, &mockBillingStore{})

	ctx := context.Background()
	for range 100 {
		_ = enforcer.CheckMonthlyRunLimit(ctx, "org_alert")
	}

	resp, err := svc.GetCurrentUsage(ctx, "org_alert")
	require.NoError(t,
		err)

	for _, alert := range resp.Alerts {
		assert.NotEqual(t,
			"monthly_runs",
			alert.
				Dimension,
		)

	}
}

func TestUsageService_GetUsageHistory_IncludesRunAndCostUsage(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{
				PeriodDate:       time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				RunsCount:        4,
				ComputeCostMicro: 2_500_000,
			},
			{
				PeriodDate:       time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
				RunsCount:        2,
				ComputeCostMicro: 500_000,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	history, err := svc.GetUsageHistory(context.Background(), "org-usage", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	require.NoError(t,
		err)
	require.Len(t, history,
		2)
	require.EqualValues(t, 4, history[0].RunsCount)
	require.EqualValues(t, 2_500_000, history[0].SpendMicro)

}

func TestUsageService_GetUsageForecast_UsesSpend(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{
				PeriodDate:       now.AddDate(0, 0, -1),
				RunsCount:        10,
				ComputeCostMicro: 2_000_000,
			},
			{
				PeriodDate:       now,
				RunsCount:        20,
				ComputeCostMicro: 4_000_000,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-forecast")
	require.NoError(t,
		err)

	assertFloatApprox(t, forecast.ProjectedMonthlySpendUsd, 90)
}

func TestUsageService_GetSpendingLimit_FreeTierWithoutSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			"org_free": 2_500_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetSpendingLimit(context.Background(), "org_free")
	require.NoError(t,
		err)
	require.Equal(t,
		"free", resp.
			PlanTier,
	)
	require.Equal(t,
		"reject", resp.
			LimitAction,
	)
	require.True(t, resp.
		IsHardCapped,
	)

	assertFloatApprox(t, resp.CurrentSpendUsd, 2.5)
	assertFloatApprox(t, resp.OverageSpendUsd, 2.5)
}

func TestUsageService_GetSpendingLimit_FreeTierWithSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_free": {OrgID: "org_free", PlanTier: "free", Status: "active", SpendingLimitMicrousd: -1, OverageDisabled: true},
		},
		periodSpendByOrg: map[string]int64{
			"org_free": 1_250_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetSpendingLimit(context.Background(), "org_free")
	require.NoError(t,
		err)
	require.Equal(t,
		"free", resp.
			PlanTier,
	)
	require.Equal(t,
		"reject", resp.
			LimitAction,
	)
	require.True(t, resp.
		IsHardCapped,
	)

	assertFloatApprox(t, resp.CurrentSpendUsd, 1.25)
	assertFloatApprox(t, resp.OverageSpendUsd, 1.25)
}

func TestUsageService_SetSpendingLimit_FreeTierWithoutSubscription(t *testing.T) {
	t.Parallel()

	svc, _ := newUsageServiceTest(t, &mockBillingStore{})

	err := svc.SetSpendingLimit(context.Background(), "org_free", 5_000_000, "notify")
	require.Error(t,
		err)
	require.Equal(t,
		"spending limits are not available on the Free plan",

		err.Error())

}

func TestUsageService_SetSpendingLimit_NegativeValue_Rejected(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", -1, "notify")
	require.Error(t,
		err)
	require.Equal(t,
		"spending limit must be non-negative",

		err.
			Error(),
	)

}

func TestUsageService_SetSpendingLimit_NegativeLargeValue_Rejected(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", -999999999, "reject")
	require.Error(t,
		err)

}

func TestUsageService_SetSpendingLimit_ZeroValue_Allowed(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", 0, "reject")
	require.NoError(t,
		err)

}

func TestUsageService_SetSpendingLimit_ValidPositive_Allowed(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", 50_000_000, "notify")
	require.NoError(t,
		err)

}

func TestUsageService_SetSpendingLimit_RaisedAboveCurrentSpendResumesQuotaPausedJobs(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-1": 10_000_000,
		},
		unpausedCount: 2,
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", 20_000_000, "reject")
	require.NoError(t,
		err)
	require.Equal(t,
		"org-1", store.
			unpausedOrgID,
	)
	require.Equal(t,
		"quota_exceeded",
		store.
			unpausedReason,
	)

}

func TestUsageService_SetSpendingLimit_NotifyActionResumesQuotaPausedJobs(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "scale", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-1": 50_000_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", 10_000_000, "notify")
	require.NoError(t,
		err)
	require.Equal(t,
		"org-1", store.
			unpausedOrgID,
	)
	require.Equal(t,
		"quota_exceeded",
		store.
			unpausedReason,
	)

}

func TestUsageService_SetSpendingLimit_StillAtRejectingCapDoesNotResume(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-1": 25_000_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", 20_000_000, "reject")
	require.NoError(t,
		err)
	require.Equal(t,
		"", store.unpausedOrgID,
	)

}

func TestUsageService_SetOverageEnabled_DisablePaidPlanStoresFlag(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetOverageEnabled(context.Background(), "org-1", false)
	require.NoError(t,
		err)
	require.Equal(t,
		"org-1", store.
			lastOverageDisabledOrg,
	)
	require.True(t, store.
		lastOverageDisabled,
	)

}

func TestUsageService_SetOverageEnabled_EnableResumesQuotaPausedJobs(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active", OverageDisabled: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetOverageEnabled(context.Background(), "org-1", true)
	require.NoError(t,
		err)
	require.False(t,
		store.lastOverageDisabled,
	)
	require.False(t,
		store.unpausedOrgID !=
			"org-1" ||
			store.unpausedReason !=
				"quota_exceeded")

}

func TestUsageService_SetOverageEnabled_FreeRequiresPaymentMethod(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-free": {OrgID: "org-free", PlanTier: "free", Status: "active", OverageDisabled: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetOverageEnabled(context.Background(), "org-free", true)
	require.Error(t,
		err)
	require.Equal(t,
		"free overage requires a payment method on file",

		err.
			Error())

}

func TestUsageService_SetOverageEnabled_FreeWithPaymentMethodAllowed(t *testing.T) {
	t.Parallel()

	customerID := "cus_free_card"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-free": {OrgID: "org-free", PlanTier: "free", Status: "active", StripeCustomerID: &customerID, OverageDisabled: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetOverageEnabled(context.Background(), "org-free", true)
	require.NoError(t,
		err)
	require.False(t,
		store.lastOverageDisabled,
	)

}

func TestUsageService_SetSpendingLimit_InvalidAction_Rejected(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.SetSpendingLimit(context.Background(), "org-1", 10_000_000, "invalid")
	require.Error(t,
		err)

}

func TestRecommendPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runs    int64
		compute int64
		want    string
	}{
		{"low_usage", 1000, 0, "free"},
		{"moderate", 200000, 10_000_000, "starter"},
		{"high", 1000000, 30_000_000, "pro"},
		{"scale_range", 5000000, 200_000_000, "scale"},
		{"business_range", 8000000, 400_000_000, "business"},
		{"very_high", 10000000, 600_000_000, "enterprise"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := recommendPlan(tt.runs, tt.compute)
			assert.Equal(t, tt.
				want, got)

		})
	}
}

func TestSafePercent(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 50.0,
		safePercent(50,
			100))
	assert.EqualValues(t, 0.0,
		safePercent(0, 0))
	assert.EqualValues(t, 0.0,
		safePercent(100,
			-1))

}

func TestUsageService_OverageCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		spend       int64
		credit      int64
		wantOverage int64
	}{
		{"spend_exceeds_credit", 30_000_000, 19_990_000, 10_010_000},
		{"spend_equals_credit", 19_990_000, 19_990_000, 0},
		{"spend_below_credit", 10_000_000, 19_990_000, 0},
		{"zero_spend_zero_credit", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := max(tt.spend-tt.credit, 0)
			assert.Equal(t, tt.
				wantOverage,
				got)

		})
	}
}

func TestUsageService_OverageAlertForPaidPlan(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_starter": {OrgID: "org_starter", PlanTier: "starter", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org_starter": 25_000_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_starter")
	require.NoError(t,
		err)
	require.False(t,
		resp.OverageMicro <=
			0)

	var foundOverageAlert bool
	for _, alert := range resp.Alerts {
		if alert.Dimension == "overage" {
			foundOverageAlert = true
			require.False(t,
				strings.Contains(alert.
					Message,
					"included credit",
				),
			)
			require.True(t, strings.Contains(alert.
				Message,
				"included run allowance",
			))

			break
		}
	}
	assert.True(t, foundOverageAlert)

}

func TestUsageService_NoOverageAlertForFreePlan(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			"org_free": 1_000_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_free")
	require.NoError(t,
		err)

	for _, alert := range resp.Alerts {
		assert.NotEqual(t,
			"overage",
			alert.Dimension,
		)

	}
}

func TestUsageService_GetCurrentUsage_FreeTierSpendIsOverage(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			"org_free": CreditFreeMicrousd,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_free")
	require.NoError(t,
		err)
	require.Equal(t,
		"free", resp.
			Plan)
	require.Equal(t,
		CreditFreeMicrousd,

		resp.PeriodSpendMicro,
	)
	require.Equal(t,
		CreditFreeMicrousd,

		resp.OverageMicro,
	)

	for _, alert := range resp.Alerts {
		require.NotEqual(
			t, "overage",
			alert.
				Dimension)

	}
}

func TestUsageService_GetCurrentUsage_FreeTierOverSpend_NoOvgAlert(t *testing.T) {
	t.Parallel()

	// Free tier does not emit the paid-plan overage alert even with high spend.
	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			"org_free": CreditFreeMicrousd + 250_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_free")
	require.NoError(t,
		err)

	for _, alert := range resp.Alerts {
		require.NotEqual(
			t, "overage",
			alert.
				Dimension)

	}
}

func TestUsageService_GetEmailPreferences(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-email": {OrgID: "org-email", PlanTier: "pro", Status: "active", MonthlyUsageEmail: false},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetEmailPreferences(context.Background(), "org-email")
	require.NoError(t,
		err)
	assert.Equal(t, false,
		resp.MonthlyUsageEmail,
	)

}

func TestUsageService_GetEmailPreferences_NotFound(t *testing.T) {
	t.Parallel()

	svc, _ := newUsageServiceTest(t, &mockBillingStore{})

	resp, err := svc.GetEmailPreferences(context.Background(), "org-missing")
	require.NoError(t,
		err)
	assert.Equal(t, true,
		resp.MonthlyUsageEmail,
	)

}

func TestUsageService_UpdateEmailPreferences(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.UpdateEmailPreferences(context.Background(), "org-update", true)
	require.NoError(t,
		err)

}

func TestStddev_Identical(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		stddev([]float64{5,
			5, 5, 5}),
	)

}

func TestBuildAlerts_ExactThresholdBoundaries(t *testing.T) {
	t.Parallel()

	svc, _ := newUsageServiceTest(t, &mockBillingStore{})

	tests := []struct {
		name     string
		percent  float64
		wantSev  string
		wantType string
	}{
		{"at_100", 100.0, "limit_reached", "limit_reached"},
		{"at_95", 95.0, "critical", "approaching_limit"},
		{"at_85", 85.0, "warning", "approaching_limit"},
		{"at_70", 70.0, "info", "approaching_limit"},
		{"below_70", 69.9, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			usage := UsageDimensions{
				MonthlyRuns: UsageDimension{Used: 0, Limit: 100, Percent: tt.percent},
			}
			alerts := svc.buildAlerts(usage)
			if tt.wantSev == "" {
				for _, a := range alerts {
					assert.NotEqual(t,
						"monthly_runs",
						a.
							Dimension)

				}
			} else {
				var found bool
				for _, a := range alerts {
					if a.Dimension == "monthly_runs" {
						found = true
						assert.Equal(t, tt.
							wantSev, a.
							Severity,
						)
						assert.Equal(t, tt.
							wantType,
							a.Type)

					}
				}
				assert.True(t, found)

			}
		})
	}
}

func TestUsageService_SetAnomalyConfig_Valid(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetAnomalyConfig(context.Background(), "org-1", 2.0, 8.0)
	require.NoError(t,
		err)

}

func TestUsageService_SetAnomalyConfig_WarningTooLow(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetAnomalyConfig(context.Background(), "org-1", 1.0, 5.0)
	require.Error(t,
		err)

}

func TestUsageService_SetAnomalyConfig_CriticalBelowWarning(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetAnomalyConfig(context.Background(), "org-1", 3.0, 3.0)
	require.Error(t,
		err)

}

func TestUsageService_SetProjectBudget_Valid(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetProjectBudget(context.Background(), "proj-1", 5_000_000, "reject")
	require.NoError(t,
		err)

}

func TestUsageService_SetProjectBudget_AcceptsBlockAction(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	svc, _ := newUsageServiceTest(t, store)
	require.NoError(t,
		svc.SetProjectBudget(context.
			Background(), "proj-1",

			5_000_000, "block"))

}

func TestUsageService_SetProjectBudget_InvalidAction(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetProjectBudget(context.Background(), "proj-1", 5_000_000, "invalid")
	require.Error(t,
		err)

}

func TestUsageService_SetProjectBudget_NegativeNormalized(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetProjectBudget(context.Background(), "proj-1", -5, "notify")
	require.NoError(t,
		err)

}

func TestUsageService_GetProjectBudget_DefaultNoBudget(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	resp, err := svc.GetProjectBudget(context.Background(), "proj-1")
	require.NoError(t,
		err)
	assert.Equal(t, "proj-1",
		resp.
			ProjectID,
	)
	assert.EqualValues(t, -1, resp.MonthlyBudgetMicro)
	assert.EqualValues(t, 0,
		resp.PercentUsed,
	)

}

func TestUsageService_GetAnomalyConfig_DefaultsForMissingSubscription(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	resp, err := svc.GetAnomalyConfig(context.Background(), "org-none")
	require.NoError(t,
		err)
	assert.Equal(t, spikeWarning,

		resp.WarningThreshold,
	)
	assert.Equal(t, spikeCritical,

		resp.CriticalThreshold,
	)

}

func TestUsageService_GetAnomalyConfig_WithSubscription_ZeroThresholds(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetAnomalyConfig(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.Equal(t, spikeWarning,

		resp.WarningThreshold,
	)

}

func TestUsageService_GetAnomalyConfig_CustomThresholdsFromSubscription(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {
				OrgID:                    "org-1",
				PlanTier:                 "pro",
				Status:                   "active",
				AnomalyThresholdWarning:  2.5,
				AnomalyThresholdCritical: 8.0,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetAnomalyConfig(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.EqualValues(t, 2.5,
		resp.WarningThreshold,
	)
	assert.EqualValues(t, 8.0,
		resp.CriticalThreshold,
	)

}

func TestUsageService_GetCurrentUsage_PaymentStatus(t *testing.T) {
	t.Parallel()
	gracePeriod := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {
				OrgID:          "org-1",
				PlanTier:       "starter",
				Status:         "active",
				PaymentStatus:  "past_due",
				GracePeriodEnd: &gracePeriod,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.Equal(t, "past_due",
		resp.
			PaymentStatus,
	)
	require.NotNil(t,
		resp.GracePeriodEnd,
	)
	assert.NotEqual(t,
		"", *resp.
			GracePeriodEnd,
	)

}

func TestUsageService_GetCurrentUsage_CreditBoundary(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-s": CreditStarterMicrousd,
		},
	}
	svc, _ := newUsageServiceTest(t, store)
	resp, err := svc.GetCurrentUsage(context.Background(), "org-s")
	require.NoError(t,
		err)
	assert.Equal(t, CreditStarterMicrousd,

		resp.OverageMicro,
	)

}

func TestUsageService_GetUsageForecast_ZeroHistory(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	forecast, err := svc.GetUsageForecast(context.Background(), "org-empty")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		forecast.ProjectedMonthlyRuns,
	)
	assert.EqualValues(t, 0,
		forecast.ProjectedMonthlySpendUsd,
	)

}

func TestUsageService_GetUsageForecast_DaysUntilMonthlyRunLimit(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-monthly": {OrgID: "org-monthly", PlanTier: "starter", Status: "active"},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 10, ComputeCostMicro: 100_000},
			{PeriodDate: now, RunsCount: 10, ComputeCostMicro: 100_000},
		},
	}
	svc, enforcer := newUsageServiceTest(t, store)
	require.NoError(t,
		enforcer.rdb.
			Set(context.
				Background(), monthlyRunKey("org-monthly", now), "49980",

				time.Hour).Err())

	forecast, err := svc.GetUsageForecast(context.Background(), "org-monthly")
	require.NoError(t,
		err)
	assert.EqualValues(t, 2,
		forecast.DaysUntilLimit,
	)

}
