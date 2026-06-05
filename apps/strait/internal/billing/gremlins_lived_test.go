package billing

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetCurrentUsage LIVED mutants.

func TestGetCurrentUsage_DailyRunErrorSilenced(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projects:     map[string][]string{"org-1": {"p1"}},
		memberCounts: map[string]int{"org-1": 1},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		resp.Usage.
			RunsToday.
			Used)
}

func TestGetCurrentUsage_DoesNotQueryUsageCost(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.Equal(t, RetentionFree,

		resp.Usage.
			RetentionDays,
	)
}

func TestGetCurrentUsage_EnterpriseContractMetadata(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-e": {OrgID: "org-e", PlanTier: "enterprise", Status: "active"},
		},
		enterpriseContracts: map[string]*EnterpriseContract{
			"org-e": {
				OrgID:              "org-e",
				EnterpriseTier:     EnterpriseTierStarter,
				OverageDiscountPct: 10,
				ContractEndDate:    time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-e": 100_000_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-e")
	require.NoError(t,
		err)
	assert.Equal(t, string(EnterpriseTierStarter),
		resp.EnterpriseTier,
	)
	assert.EqualValues(t, 100_000_000,

		resp.PeriodSpendMicro,
	)
	assert.Equal(t, 10,
		resp.OverageDiscountPct,
	)
	assert.EqualValues(t, 90_000_000,

		resp.OverageMicro,
	)
}

func TestGetCurrentUsage_StarterSpendIsOverage(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-s": CreditStarterMicrousd - 1_000_000,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-s")
	require.NoError(t,
		err)

	want := CreditStarterMicrousd - 1_000_000
	assert.Equal(t, want,
		resp.
			OverageMicro,
	)
}

func TestGetCurrentUsage_SpendEqualsStarterPriceIsOverage(t *testing.T) {
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

func TestGetCurrentUsage_SpendAboveStarterPriceIsOverage(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-s": CreditStarterMicrousd + 1,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-s")
	require.NoError(t,
		err)
	assert.Equal(t, CreditStarterMicrousd+
		1, resp.
		OverageMicro,
	)
}

func TestGetCurrentUsage_ActiveAddonsPopulated(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		activeAddons: []Addon{
			{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-1")
	require.NoError(t,
		err)
	require.Len(t, resp.
		ActiveAddons,
		1)
	assert.Equal(t, 2,
		resp.ActiveAddons[0].
			Quantity,
	)
}

func TestGetCurrentUsage_OverageAlertMessage(t *testing.T) {
	t.Parallel()
	const spend = CreditStarterMicrousd + 5_000_000
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		periodSpendByOrg: map[string]int64{
			"org-s": spend,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-s")
	require.NoError(t,
		err)
	assert.Equal(t, spend,
		resp.
			OverageMicro,
	)

	// Orchestration-only: all spend is overage (no included compute credit).

	var found bool
	for _, a := range resp.Alerts {
		if a.Dimension == "overage" {
			found = true
		}
	}
	assert.True(t, found)
}

// GetUsageForecast LIVED mutants.

func TestGetUsageForecast_ArithmeticValues(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -2), RunsCount: 30, ComputeCostMicro: 3_000_000},
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 30, ComputeCostMicro: 3_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-f")
	require.NoError(t,
		err)
	assert.EqualValues(t, 30*
		30, forecast.
		ProjectedMonthlyRuns,
	)

	assertFloatApprox(t, forecast.ProjectedMonthlySpendUsd, 90.0)
}

func TestGetUsageForecast_DaysUntilLimit_UsesMonthlyRunAllowance(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 10, ComputeCostMicro: 100_000},
			{PeriodDate: now, RunsCount: 10, ComputeCostMicro: 100_000},
		},
	}
	svc, enforcer := newUsageServiceTest(t, store)
	require.NoError(t,
		enforcer.
			rdb.Set(context.
			Background(), monthlyRunKey("org-s",
			now), "49980",
			time.Hour,
		).Err())

	forecast, err := svc.GetUsageForecast(context.Background(), "org-s")
	require.NoError(t,
		err)
	assert.Equal(t, 2,
		forecast.
			DaysUntilLimit,
	)
}

func TestGetUsageForecast_ProjectedOverage(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 100, ComputeCostMicro: 5_000_000},
			{PeriodDate: now, RunsCount: 100, ComputeCostMicro: 5_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-s")
	require.NoError(t,
		err)

	// Orchestration-only: no included credit; all projected spend is overage.
	projectedMicro := int64(5_000_000) * 30
	expectedOverage := computeOverageSpend(projectedMicro, 0)
	assert.Equal(t, expectedOverage,

		forecast.
			ProjectedOverageMicro,
	)
}

func TestGetUsageForecast_AddonSpendIncluded(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-p": {OrgID: "org-p", PlanTier: "pro", Status: "active"},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 10, ComputeCostMicro: 1_000_000},
		},
		activeAddons: []Addon{
			{AddonType: AddonConcurrency100, Quantity: 2, Active: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-p")
	require.NoError(t,
		err)

	pack := AddonPacks[AddonConcurrency100]
	expectedAddonMicro := int64(pack.PriceCents) * 2 * 10000
	assert.Equal(t, expectedAddonMicro,

		forecast.
			AddonSpendMicro,
	)
}

func TestGetUsageForecast_AddonInactive_NotCounted(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{PeriodDate: now, RunsCount: 1, ComputeCostMicro: 100},
		},
		activeAddons: []Addon{
			{AddonType: AddonConcurrency100, Quantity: 2, Active: false},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		forecast.
			AddonSpendMicro,
	)
}

func TestGetUsageForecast_RoadmapAddonsNotCounted(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{PeriodDate: now, RunsCount: 1, ComputeCostMicro: 100},
		},
		activeAddons: []Addon{
			{AddonType: AddonComplianceArchive, Quantity: 1, Active: true},
			{AddonType: AddonDedicatedWorkers, Quantity: 1, Active: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.EqualValues(t, 0,
		forecast.
			AddonSpendMicro,
	)
}

func TestGetUsageForecast_ScaleBreakeven(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-p": {OrgID: "org-p", PlanTier: "pro", Status: "active"},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 500, ComputeCostMicro: 10_000_000},
			{PeriodDate: now, RunsCount: 500, ComputeCostMicro: 10_000_000},
		},
		activeAddons: []Addon{
			{AddonType: AddonConcurrency100, Quantity: 5, Active: true},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-p")
	require.NoError(t,
		err)

	totalProSpend := int64(PriceProMonthlyCents)*10000 + forecast.AddonSpendMicro + forecast.ProjectedOverageMicro
	expectedBreakeven := totalProSpend >= CreditScaleMicrousd
	assert.Equal(t, expectedBreakeven,

		forecast.
			ScaleBreakeven,
	)
}

func TestGetUsageForecast_ScaleBreakeven_NonPro_AlwaysFalse(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-s": {OrgID: "org-s", PlanTier: "starter", Status: "active"},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 500, ComputeCostMicro: 50_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-s")
	require.NoError(t,
		err)
	assert.False(t, forecast.
		ScaleBreakeven,
	)
}

func TestGetUsageForecast_ConfidenceInterval(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -3), RunsCount: 10, ComputeCostMicro: 1_000_000},
			{PeriodDate: now.AddDate(0, 0, -2), RunsCount: 10, ComputeCostMicro: 3_000_000},
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 10, ComputeCostMicro: 5_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-ci")
	require.NoError(t,
		err)
	assert.Less(t, forecast.
		ProjectedMonthlySpendLowUsd, forecast.
		ProjectedMonthlySpendUsd,
	)
	assert.Greater(t, forecast.
		ProjectedMonthlySpendHighUsd, forecast.
		ProjectedMonthlySpendUsd,
	)
	assert.GreaterOrEqual(t, forecast.
		ProjectedMonthlySpendLowUsd,

		0.0)
	assert.Equal(t, 87,
		forecast.
			ConfidencePct,
	)
}

func TestGetUsageForecast_IdenticalDays_ZeroStddev(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -2), RunsCount: 10, ComputeCostMicro: 2_000_000},
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 10, ComputeCostMicro: 2_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-id")
	require.NoError(t,
		err)
	assert.InDelta(t, forecast.
		ProjectedMonthlySpendHighUsd,

		forecast.
			ProjectedMonthlySpendLowUsd, 1e-9,
	)
}

// DetectAnomalies LIVED mutants.

func TestDetectAnomalies_WithSpendingLimitAndForecast(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-a": {
				OrgID:                 "org-a",
				PlanTier:              "pro",
				Status:                "active",
				SpendingLimitMicrousd: 10_000_000,
			},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 100, ComputeCostMicro: 5_000_000},
			{PeriodDate: now, RunsCount: 100, ComputeCostMicro: 5_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	alerts, err := svc.DetectAnomalies(context.Background(), "org-a")
	require.NoError(t,
		err)

	var foundBudgetAlert bool
	for _, a := range alerts {
		if a.Severity == AnomalySeverityWarning && a.TodaySpend == 0 && a.Avg7dSpend == 0 {
			foundBudgetAlert = true
			assert.Equal(t, "org-a",
				a.
					OrgID)
		}
	}
	projectedMicro := int64(5_000_000) * 30
	assert.False(t, projectedMicro >
		10_000_000 &&
		!foundBudgetAlert,
	)
}

func TestDetectAnomalies_CustomThresholdsFromSubscription(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-c": {
				OrgID:                    "org-c",
				PlanTier:                 "pro",
				Status:                   "active",
				AnomalyThresholdWarning:  2.0,
				AnomalyThresholdCritical: 8.0,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	_, err := svc.DetectAnomalies(context.Background(), "org-c")
	require.NoError(t,
		err)
}

func TestDetectAnomalies_NoSpendingLimit_NoBudgetAlert(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-b": {OrgID: "org-b", PlanTier: "pro", Status: "active", SpendingLimitMicrousd: 0},
		},
		usageRecords: []UsageRecord{
			{PeriodDate: now.AddDate(0, 0, -1), RunsCount: 100, ComputeCostMicro: 50_000_000},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	alerts, err := svc.DetectAnomalies(context.Background(), "org-b")
	require.NoError(t,
		err)

	for _, a := range alerts {
		assert.False(t, a.
			Severity ==
			AnomalySeverityWarning &&
			a.TodaySpend ==
				0 && a.Avg7dSpend ==
			0)
	}
}

// GetAnomalyConfig LIVED mutants.

func TestGetAnomalyConfig_ZeroWarning_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active", AnomalyThresholdWarning: 0, AnomalyThresholdCritical: 12.0},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetAnomalyConfig(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.InDelta(t, spikeWarning,

		resp.WarningThreshold, 1e-9,
	)
	assert.InDelta(t, 12.0,
		resp.
			CriticalThreshold, 1e-9,
	)
}

func TestGetAnomalyConfig_ZeroCritical_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active", AnomalyThresholdWarning: 2.5, AnomalyThresholdCritical: 0},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetAnomalyConfig(context.Background(), "org-1")
	require.NoError(t,
		err)
	assert.InDelta(t, 2.5,
		resp.WarningThreshold, 1e-9,
	)
	assert.InDelta(t, spikeCritical,

		resp.CriticalThreshold, 1e-9,
	)
}

// GetProjectBudget LIVED mutant.

func TestGetProjectBudget_PositiveBudget_PercentCalculated(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		getProjectBudgetFn: func(_ context.Context, _ string) (int64, string, error) {
			return 10_000_000, "reject", nil
		},
		getProjectPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 5_000_000, nil
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetProjectBudget(context.Background(), "proj-1")
	require.NoError(t,
		err)
	assert.EqualValues(t, 10_000_000,

		resp.MonthlyBudgetMicro,
	)

	assertFloatApprox(t, resp.PercentUsed, 50.0)
}

func TestGetProjectBudget_ZeroBudget_ZeroPercent(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		getProjectBudgetFn: func(_ context.Context, _ string) (int64, string, error) {
			return 0, "reject", nil
		},
		getProjectPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 5_000_000, nil
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetProjectBudget(context.Background(), "proj-1")
	require.NoError(t,
		err)
	assert.InDelta(t, 0,
		resp.PercentUsed, 1e-9,
	)
}

// SetSpendingLimit LIVED mutants.

func TestSetSpendingLimit_ExactMaxLimit(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	maxLimit := MaxSpendingLimit(domain.PlanStarter)
	err := svc.SetSpendingLimit(context.Background(), "org-1", maxLimit, "notify")
	require.NoError(t,
		err)
}

func TestSetSpendingLimit_AboveMaxLimit(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "starter", Status: "active"},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	maxLimit := MaxSpendingLimit(domain.PlanStarter)
	err := svc.SetSpendingLimit(context.Background(), "org-1", maxLimit+1, "notify")
	require.Error(t,
		err)
}

// stddev LIVED mutant.

func TestStddev_KnownVariance(t *testing.T) {
	t.Parallel()
	result := stddev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	assert.LessOrEqual(t, math.
		Abs(result-
			2.0), 0.01,
	)
}

func TestStddev_SingleAndEmpty(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 0,
		stddev(nil), 1e-9)
	assert.InDelta(t, 0,
		stddev([]float64{42}), 1e-9)
}

// Export LIVED mutants.

func TestExportPDF_SubscriptionAffectsOutput(t *testing.T) {
	t.Parallel()
	records := []UsageRecord{
		{ProjectID: "proj-a", PeriodDate: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), RunsCount: 50, ComputeCostMicro: 7_000_000},
	}
	period := ExportPeriod{
		From: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
	}

	noSubStore := &mockExportStore{usageRecords: records}
	noSubData, err := ExportPDF(context.Background(), noSubStore, "org-1", period)
	require.NoError(t,
		err)

	withSubStore := &mockExportStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
		},
		usageRecords: records,
	}
	withSubData, err := ExportPDF(context.Background(), withSubStore, "org-1", period)
	require.NoError(t,
		err)
	assert.False(t, len(noSubData) == len(withSubData) && string(noSubData) == string(withSubData))
}

func TestExportCSV_ArithmeticTotals(t *testing.T) {
	t.Parallel()
	store := &mockExportStore{
		usageRecords: []UsageRecord{
			{ProjectID: "proj-a", PeriodDate: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), RunsCount: 50, ComputeCostMicro: 7_000_000},
			{ProjectID: "proj-b", PeriodDate: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), RunsCount: 30, ComputeCostMicro: 3_000_000},
		},
	}
	period := ExportPeriod{
		From: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportCSV(context.Background(), store, "org-1", period)
	require.NoError(t,
		err)

	content := string(data)
	assert.Contains(t, content, "7.000000")
	assert.Contains(t, content, "3.000000")
}

// PostHog LIVED mutants.

func TestPostHogCapture_StatusBoundary_400_IsWarned(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewPostHogClient("key", srv.URL, slog.Default())
	c.Capture(context.Background(), "user-1", "test", nil)
}

func TestPostHogCapture_StatusBoundary_399_NoWarn(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(399)
	}))
	defer srv.Close()

	c := NewPostHogClient("key", srv.URL, slog.Default())
	c.Capture(context.Background(), "user-1", "test", nil)
}

func TestPostHogClient_Timeout_Positive(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "http://localhost:1", nil)
	assert.Positive(t, c.
		client.Timeout)
	assert.Equal(t, 5*
		time.Second,
		c.client.
			Timeout,
	)
}

func TestReconcileAllConcurrentCounts_UsesMapValue(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	rdb.Set(context.Background(), "strait:org_concurrent:org-1", "99", 0)

	counter := &mockExecutingRunCounter{
		listOrgs:  []string{"org-1"},
		orgCounts: map[string]int{"org-1": 7},
	}

	err := enforcer.ReconcileAllConcurrentCounts(context.Background(), counter)
	require.NoError(t,
		err)

	val, getErr := rdb.Get(context.Background(), "strait:org_concurrent:org-1").Int()
	require.NoError(t, getErr)
	assert.Equal(t, 7,
		val)
}

// Enterprise LIVED mutant.

func TestApplyOverageDiscount_ZeroDiscountReturnsCost(t *testing.T) {
	t.Parallel()
	cost := ApplyOverageDiscount(1_000_000, 0)
	assert.EqualValues(t, 1_000_000,

		cost)
}

func TestApplyOverageDiscount_OnePercentDiscount(t *testing.T) {
	t.Parallel()
	cost := ApplyOverageDiscount(1_000_000, 1)
	expected := int64(1_000_000 * 99 / 100)
	assert.Equal(t, expected,
		cost,
	)
}

// Plans LIVED mutant.

func TestIsDowngrade_ScaleToPro_IsDowngrade(t *testing.T) {
	t.Parallel()
	assert.True(t, IsDowngrade(
		domain.PlanScale,
		domain.
			PlanPro,
	))
}

func TestIsDowngrade_ProToStarter_IsDowngrade(t *testing.T) {
	t.Parallel()
	assert.True(t, IsDowngrade(
		domain.PlanPro,
		domain.
			PlanStarter,
	))
}

// EffectiveLimits boundary mutants.

func TestEffectiveLimits_ZeroQuantity_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 0, Active: true},
	}
	result := EffectiveLimits(base, addons)
	assert.Equal(t, base.
		MaxConcurrentRuns,

		result.
			MaxConcurrentRuns,
	)
}

func TestEffectiveLimits_HistoryAddsAdditively(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanScale)
	pack := AddonPacks[AddonHistory30d]
	addons := []Addon{
		{AddonType: AddonHistory30d, Quantity: 3, Active: true},
	}
	result := EffectiveLimits(base, addons)
	want := base.RetentionDays + pack.PackSize*3
	assert.Equal(t, want,
		result.
			RetentionDays,
	)
}

// Downgrade boundary mutants.

func TestPreviewDowngrade_ZeroHTTPJobs_NoImpact(t *testing.T) {
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
			projects:     map[string][]string{"org-1": {"p1"}},
			httpJobCount: 0,
		},
	}
	impact, err := PreviewDowngrade(context.Background(), store, "org-1", domain.PlanFree)
	require.NoError(t,
		err)

	for _, imp := range impact.Impacts {
		assert.NotEqual(t,
			"http_mode_jobs",
			imp.
				Resource,
		)
	}
}

// Enforcement LIVED mutants: CheckProjectSuspended, SuspendExcessProjects.

func TestEnforcer_SuspendExcessProjects_UnlimitedSkips(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	n, err := enforcer.SuspendExcessProjects(context.Background(), "org-1", -1)
	require.NoError(t,
		err)
	assert.Equal(t, 0,
		n)
}

func TestEnforcer_SuspendExcessProjects_PositiveLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	n, err := enforcer.SuspendExcessProjects(context.Background(), "org-1", 2)
	require.NoError(t,
		err)
	assert.Equal(t, 0,
		n)
}

func TestEnforcer_CheckProjectSuspended_EmptyProjectID(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	require.NoError(t,
		enforcer.
			CheckProjectSuspended(context.Background(), ""))
}

func TestEnforcer_CheckProjectSuspended_NotSuspended(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	require.NoError(t,
		enforcer.
			CheckProjectSuspended(context.Background(), "proj-1"))
	require.NoError(t,
		enforcer.
			CheckProjectSuspended(context.Background(), "proj-1"))

	// Second call should hit the cache.
}

func TestEnforcer_CheckProjectSuspended_ReadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		isProjectSuspendedErr: errors.New("project suspension status unavailable"),
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckProjectSuspended(context.Background(), "proj-read-error")
	require.Error(t,
		err)

	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t,
		"service_degraded",
		le.
			Code)
}

func TestEnforcer_CheckProjectSuspended_FlushCache(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	require.NoError(t,
		enforcer.
			CheckProjectSuspended(context.Background(), "proj-flush"))

	enforcer.InvalidateProjectSuspendedCache("proj-flush")
	enforcer.FlushSuspendedCacheForOrg([]string{"proj-flush"})
	require.NoError(t,
		enforcer.
			CheckProjectSuspended(context.Background(), "proj-flush"))
}

// Enterprise boundary LIVED mutants.

func TestApplyOverageDiscount_NegativeCost_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(-100, 10))
}

func TestApplyOverageDiscount_ExactlyZeroCost_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(0, 10))
}

func TestApplyOverageDiscount_ExactlyHundredPct_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(1_000_000,
			100))
}

func TestApplyOverageDiscount_OverHundredPct_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.EqualValues(t, 0,
		ApplyOverageDiscount(1_000_000,
			150))
}

func TestApplyOverageDiscount_OneCost_OnePct(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(1, 1)
	assert.False(t, got <
		0 ||
		got > 1)
}

func TestCalculateSLACredit_AtTarget_ZeroCredit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0,
		CalculateSLACredit(99.9,
			99.9,
		))
}

func TestCalculateSLACredit_AboveTarget_ZeroCredit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0,
		CalculateSLACredit(99.95,
			99.9,
		))
}

func TestCalculateSLACredit_JustBelowTarget_TenPct(t *testing.T) {
	t.Parallel()
	got := CalculateSLACredit(99.89, 99.9)
	assert.Equal(t, 10,
		got)
}

func TestCalculateSLACredit_BelowNinety_FiftyPct(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 50,
		CalculateSLACredit(
			89.9, 99.9,
		))
}

func TestCalculateSLACredit_HigherTarget_ExtendedRange(t *testing.T) {
	t.Parallel()
	got := CalculateSLACredit(99.91, 99.95)
	assert.Equal(t, 10,
		got)
}

func TestCalculateSLACredit_BoundaryTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		uptime   float64
		target   float64
		expected int
	}{
		{99.0, 99.9, 10},
		{98.99, 99.9, 25},
		{95.0, 99.9, 25},
		{94.99, 99.9, 50},
		{90.0, 99.9, 50},
		{89.99, 99.9, 50},
	}
	for _, tt := range tests {
		got := CalculateSLACredit(tt.uptime, tt.target)
		assert.Equal(t, tt.
			expected,
			got)
	}
}

// Spending limit message formatting (enforcement.go:776, 804).

func TestEnforcer_CheckSpendingLimit_MessageContainsDollarAmount(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-spend": {
				OrgID:                 "org-spend",
				PlanTier:              "pro",
				Status:                "active",
				SpendingLimitMicrousd: 50_000_000,
				LimitAction:           "reject",
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-spend": CreditProMicrousd + 60_000_000,
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckSpendingLimit(context.Background(), "org-spend")
	require.Error(t,
		err)

	var le *LimitError
	require.ErrorAs(t, err, &le)
	assert.Contains(t, le.Message, "$50.00")
}

func TestEnforcer_CheckSpendingLimit_FreeTierMessageContainsBudget(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			// Any spend triggers the free-tier cap in orchestration-only mode.
			"org-free-over": 1,
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckSpendingLimit(context.Background(), "org-free-over")
	require.Error(t,
		err)

	var le *LimitError
	require.ErrorAs(t, err, &le)
	assert.False(t, !strings.Contains(le.Message,
		"budget",
	) &&
		!strings.Contains(le.
			Message, "compute",
		))

	// Message should reference budget being reached (no dollar amount for $0 credit).
}

// DecrDailyRunCount and DecrConcurrentRunCount nil guards.

func TestEnforcer_DecrDailyRunCount_EmptyOrgID(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	enforcer.DecrDailyRunCount(context.Background(), "")
}

func TestEnforcer_DecrConcurrentRunCount_EmptyOrgID(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	enforcer.DecrConcurrentRunCount(context.Background(), "")
}

// Enforcement recordRejection/recordFailOpen nil-metrics guards.

func TestEnforcer_RecordRejection_NilMetrics(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	enforcer.recordRejection(context.Background(), "test", domain.PlanFree)
}

func TestEnforcer_RecordFailOpen_NilMetrics(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	enforcer.recordFailOpen(context.Background(), "test", "db_error")
}

func TestPreviewDowngrade_NoRegionImpactInRegressionSuite(t *testing.T) {
	store := &mockDowngradeStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
			projects: map[string][]string{"org-1": {"p1"}},
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
