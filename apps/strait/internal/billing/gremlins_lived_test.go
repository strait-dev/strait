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
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.RunsToday.Used != 0 {
		t.Errorf("RunsToday.Used = %d, want 0 (error silenced to 0)", resp.Usage.RunsToday.Used)
	}
}

func TestGetCurrentUsage_DoesNotQueryUsageCost(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.RetentionDays != RetentionFree {
		t.Errorf("RetentionDays = %d, want %d", resp.Usage.RetentionDays, RetentionFree)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.EnterpriseTier != string(EnterpriseTierStarter) {
		t.Errorf("EnterpriseTier = %q, want %q", resp.EnterpriseTier, EnterpriseTierStarter)
	}
	if resp.PeriodSpendMicro != 100_000_000 {
		t.Errorf("PeriodSpendMicro = %d, want 100000000", resp.PeriodSpendMicro)
	}
	if resp.OverageDiscountPct != 10 {
		t.Errorf("OverageDiscountPct = %d, want 10", resp.OverageDiscountPct)
	}
	if resp.OverageMicro != 90_000_000 {
		t.Errorf("OverageMicro = %d, want 90000000", resp.OverageMicro)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	want := CreditStarterMicrousd - 1_000_000
	if resp.OverageMicro != want {
		t.Errorf("OverageMicro = %d, want %d", resp.OverageMicro, want)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.OverageMicro != CreditStarterMicrousd {
		t.Errorf("OverageMicro = %d, want %d", resp.OverageMicro, CreditStarterMicrousd)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.OverageMicro != CreditStarterMicrousd+1 {
		t.Errorf("OverageMicro = %d, want %d (all spend is overage)", resp.OverageMicro, CreditStarterMicrousd+1)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ActiveAddons) != 1 {
		t.Fatalf("ActiveAddons len = %d, want 1", len(resp.ActiveAddons))
	}
	if resp.ActiveAddons[0].Quantity != 2 {
		t.Errorf("addon quantity = %d, want 2", resp.ActiveAddons[0].Quantity)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	// Orchestration-only: all spend is overage (no included compute credit).
	if resp.OverageMicro != spend {
		t.Errorf("OverageMicro = %d, want %d", resp.OverageMicro, spend)
	}
	var found bool
	for _, a := range resp.Alerts {
		if a.Dimension == "overage" {
			found = true
		}
	}
	if !found {
		t.Error("expected overage alert for paid plan with overage spend")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	if forecast.ProjectedMonthlyRuns != 30*30 {
		t.Errorf("ProjectedMonthlyRuns = %d, want %d", forecast.ProjectedMonthlyRuns, 30*30)
	}
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
	if err := enforcer.rdb.Set(context.Background(), monthlyRunKey("org-s", now), "49980", time.Hour).Err(); err != nil {
		t.Fatal(err)
	}

	forecast, err := svc.GetUsageForecast(context.Background(), "org-s")
	if err != nil {
		t.Fatal(err)
	}
	if forecast.DaysUntilLimit != 2 {
		t.Errorf("DaysUntilLimit = %d, want 2 based on monthly run allowance", forecast.DaysUntilLimit)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	// Orchestration-only: no included credit; all projected spend is overage.
	projectedMicro := int64(5_000_000) * 30
	expectedOverage := computeOverageSpend(projectedMicro, 0)
	if forecast.ProjectedOverageMicro != expectedOverage {
		t.Errorf("ProjectedOverageMicro = %d, want %d", forecast.ProjectedOverageMicro, expectedOverage)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	pack := AddonPacks[AddonConcurrency100]
	expectedAddonMicro := int64(pack.PriceCents) * 2 * 10000
	if forecast.AddonSpendMicro != expectedAddonMicro {
		t.Errorf("AddonSpendMicro = %d, want %d", forecast.AddonSpendMicro, expectedAddonMicro)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if forecast.AddonSpendMicro != 0 {
		t.Errorf("AddonSpendMicro = %d, want 0 for inactive addon", forecast.AddonSpendMicro)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if forecast.AddonSpendMicro != 0 {
		t.Errorf("AddonSpendMicro = %d, want 0 for roadmap addons", forecast.AddonSpendMicro)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	totalProSpend := int64(PriceProMonthlyCents)*10000 + forecast.AddonSpendMicro + forecast.ProjectedOverageMicro
	expectedBreakeven := totalProSpend >= CreditScaleMicrousd
	if forecast.ScaleBreakeven != expectedBreakeven {
		t.Errorf("ScaleBreakeven = %v, want %v (totalProSpend=%d, ScaleCredit=%d)",
			forecast.ScaleBreakeven, expectedBreakeven, totalProSpend, CreditScaleMicrousd)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if forecast.ScaleBreakeven {
		t.Error("ScaleBreakeven should be false for non-Pro tiers")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	if forecast.ProjectedMonthlySpendLowUsd >= forecast.ProjectedMonthlySpendUsd {
		t.Errorf("Low = %f should be < Projected = %f",
			forecast.ProjectedMonthlySpendLowUsd, forecast.ProjectedMonthlySpendUsd)
	}
	if forecast.ProjectedMonthlySpendHighUsd <= forecast.ProjectedMonthlySpendUsd {
		t.Errorf("High = %f should be > Projected = %f",
			forecast.ProjectedMonthlySpendHighUsd, forecast.ProjectedMonthlySpendUsd)
	}
	if forecast.ProjectedMonthlySpendLowUsd < 0 {
		t.Errorf("Low = %f should be >= 0", forecast.ProjectedMonthlySpendLowUsd)
	}
	if forecast.ConfidencePct != 87 {
		t.Errorf("ConfidencePct = %d, want 87", forecast.ConfidencePct)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if forecast.ProjectedMonthlySpendLowUsd != forecast.ProjectedMonthlySpendHighUsd {
		t.Errorf("Low (%f) != High (%f) for identical daily spend",
			forecast.ProjectedMonthlySpendLowUsd, forecast.ProjectedMonthlySpendHighUsd)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	var foundBudgetAlert bool
	for _, a := range alerts {
		if a.Severity == AnomalySeverityWarning && a.TodaySpend == 0 && a.Avg7dSpend == 0 {
			foundBudgetAlert = true
			if a.OrgID != "org-a" {
				t.Errorf("alert OrgID = %q, want org-a", a.OrgID)
			}
		}
	}
	projectedMicro := int64(5_000_000) * 30
	if projectedMicro > 10_000_000 && !foundBudgetAlert {
		t.Error("expected projected budget exceeded alert when forecast exceeds spending limit")
	}
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
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range alerts {
		if a.Severity == AnomalySeverityWarning && a.TodaySpend == 0 && a.Avg7dSpend == 0 {
			t.Error("should not have budget alert when spending limit is 0")
		}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.WarningThreshold != spikeWarning {
		t.Errorf("WarningThreshold = %f, want %f (default for zero)", resp.WarningThreshold, spikeWarning)
	}
	if resp.CriticalThreshold != 12.0 {
		t.Errorf("CriticalThreshold = %f, want 12.0", resp.CriticalThreshold)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.WarningThreshold != 2.5 {
		t.Errorf("WarningThreshold = %f, want 2.5", resp.WarningThreshold)
	}
	if resp.CriticalThreshold != spikeCritical {
		t.Errorf("CriticalThreshold = %f, want %f (default for zero)", resp.CriticalThreshold, spikeCritical)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.MonthlyBudgetMicro != 10_000_000 {
		t.Errorf("MonthlyBudgetMicro = %d, want 10000000", resp.MonthlyBudgetMicro)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.PercentUsed != 0 {
		t.Errorf("PercentUsed = %f, want 0 for zero budget", resp.PercentUsed)
	}
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
	if err != nil {
		t.Fatalf("setting spending limit at exact max should succeed: %v", err)
	}
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
	if err == nil {
		t.Fatal("setting spending limit above max should fail")
	}
}

// stddev LIVED mutant.

func TestStddev_KnownVariance(t *testing.T) {
	t.Parallel()
	result := stddev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if math.Abs(result-2.0) > 0.01 {
		t.Errorf("stddev = %f, want ~2.0", result)
	}
}

func TestStddev_SingleAndEmpty(t *testing.T) {
	t.Parallel()
	if got := stddev(nil); got != 0 {
		t.Errorf("stddev(nil) = %f, want 0", got)
	}
	if got := stddev([]float64{42}); got != 0 {
		t.Errorf("stddev([42]) = %f, want 0", got)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	withSubStore := &mockExportStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
		},
		usageRecords: records,
	}
	withSubData, err := ExportPDF(context.Background(), withSubStore, "org-1", period)
	if err != nil {
		t.Fatal(err)
	}
	if len(noSubData) == len(withSubData) && string(noSubData) == string(withSubData) {
		t.Error("expected different PDF output when subscription changes plan tier from free to pro")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "7.000000") {
		t.Error("CSV should contain row total 7.000000")
	}
	if !strings.Contains(content, "3.000000") {
		t.Error("CSV should contain row total 3.000000")
	}
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
	if c.client.Timeout <= 0 {
		t.Errorf("client timeout = %v, want > 0", c.client.Timeout)
	}
	if c.client.Timeout != 5*time.Second {
		t.Errorf("client timeout = %v, want 5s", c.client.Timeout)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	val, getErr := rdb.Get(context.Background(), "strait:org_concurrent:org-1").Int()
	if getErr != nil {
		t.Fatal(getErr)
	}
	if val != 7 {
		t.Errorf("reconciled counter = %d, want 7", val)
	}
}

// Enterprise LIVED mutant.

func TestApplyOverageDiscount_ZeroDiscountReturnsCost(t *testing.T) {
	t.Parallel()
	cost := ApplyOverageDiscount(1_000_000, 0)
	if cost != 1_000_000 {
		t.Errorf("ApplyOverageDiscount(1000000, 0) = %d, want 1000000", cost)
	}
}

func TestApplyOverageDiscount_OnePercentDiscount(t *testing.T) {
	t.Parallel()
	cost := ApplyOverageDiscount(1_000_000, 1)
	expected := int64(1_000_000 * 99 / 100)
	if cost != expected {
		t.Errorf("ApplyOverageDiscount(1000000, 1) = %d, want %d", cost, expected)
	}
}

// Plans LIVED mutant.

func TestIsDowngrade_ScaleToPro_IsDowngrade(t *testing.T) {
	t.Parallel()
	if !IsDowngrade(domain.PlanScale, domain.PlanPro) {
		t.Error("scale -> pro should be a downgrade (concurrent runs decrease)")
	}
}

func TestIsDowngrade_ProToStarter_IsDowngrade(t *testing.T) {
	t.Parallel()
	if !IsDowngrade(domain.PlanPro, domain.PlanStarter) {
		t.Error("pro -> starter should be a downgrade")
	}
}

// EffectiveLimits boundary mutants.

func TestEffectiveLimits_ZeroQuantity_Ignored(t *testing.T) {
	t.Parallel()
	base := GetPlanLimits(domain.PlanPro)
	addons := []Addon{
		{AddonType: AddonConcurrency100, Quantity: 0, Active: true},
	}
	result := EffectiveLimits(base, addons)
	if result.MaxConcurrentRuns != base.MaxConcurrentRuns {
		t.Errorf("zero quantity should be ignored: got %d, want %d",
			result.MaxConcurrentRuns, base.MaxConcurrentRuns)
	}
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
	if result.RetentionDays != want {
		t.Errorf("retention = %d, want %d", result.RetentionDays, want)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range impact.Impacts {
		if imp.Resource == "http_mode_jobs" {
			t.Error("expected no http_mode_jobs impact when count is 0")
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("SuspendExcessProjects(-1) = %d, want 0", n)
	}
}

func TestEnforcer_SuspendExcessProjects_PositiveLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	n, err := enforcer.SuspendExcessProjects(context.Background(), "org-1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("SuspendExcessProjects(2) = %d, want 0", n)
	}
}

func TestEnforcer_CheckProjectSuspended_EmptyProjectID(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	if err := enforcer.CheckProjectSuspended(context.Background(), ""); err != nil {
		t.Fatalf("empty project ID should return nil: %v", err)
	}
}

func TestEnforcer_CheckProjectSuspended_NotSuspended(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	if err := enforcer.CheckProjectSuspended(context.Background(), "proj-1"); err != nil {
		t.Fatalf("non-suspended project should return nil: %v", err)
	}

	// Second call should hit the cache.
	if err := enforcer.CheckProjectSuspended(context.Background(), "proj-1"); err != nil {
		t.Fatalf("cached non-suspended project should return nil: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected project suspension check to fail closed when status cannot be loaded")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
	}
}

func TestEnforcer_CheckProjectSuspended_FlushCache(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	if err := enforcer.CheckProjectSuspended(context.Background(), "proj-flush"); err != nil {
		t.Fatal(err)
	}
	enforcer.InvalidateProjectSuspendedCache("proj-flush")
	enforcer.FlushSuspendedCacheForOrg([]string{"proj-flush"})

	if err := enforcer.CheckProjectSuspended(context.Background(), "proj-flush"); err != nil {
		t.Fatal(err)
	}
}

// Enterprise boundary LIVED mutants.

func TestApplyOverageDiscount_NegativeCost_ReturnsZero(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(-100, 10); got != 0 {
		t.Errorf("ApplyOverageDiscount(-100, 10) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_ExactlyZeroCost_ReturnsZero(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(0, 10); got != 0 {
		t.Errorf("ApplyOverageDiscount(0, 10) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_ExactlyHundredPct_ReturnsZero(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(1_000_000, 100); got != 0 {
		t.Errorf("ApplyOverageDiscount(1000000, 100) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_OverHundredPct_ReturnsZero(t *testing.T) {
	t.Parallel()
	if got := ApplyOverageDiscount(1_000_000, 150); got != 0 {
		t.Errorf("ApplyOverageDiscount(1000000, 150) = %d, want 0", got)
	}
}

func TestApplyOverageDiscount_OneCost_OnePct(t *testing.T) {
	t.Parallel()
	got := ApplyOverageDiscount(1, 1)
	if got < 0 || got > 1 {
		t.Errorf("ApplyOverageDiscount(1, 1) = %d, want 0 or 1", got)
	}
}

func TestCalculateSLACredit_AtTarget_ZeroCredit(t *testing.T) {
	t.Parallel()
	if got := CalculateSLACredit(99.9, 99.9); got != 0 {
		t.Errorf("CalculateSLACredit(99.9, 99.9) = %d, want 0", got)
	}
}

func TestCalculateSLACredit_AboveTarget_ZeroCredit(t *testing.T) {
	t.Parallel()
	if got := CalculateSLACredit(99.95, 99.9); got != 0 {
		t.Errorf("CalculateSLACredit(99.95, 99.9) = %d, want 0", got)
	}
}

func TestCalculateSLACredit_JustBelowTarget_TenPct(t *testing.T) {
	t.Parallel()
	got := CalculateSLACredit(99.89, 99.9)
	if got != 10 {
		t.Errorf("CalculateSLACredit(99.89, 99.9) = %d, want 10", got)
	}
}

func TestCalculateSLACredit_BelowNinety_FiftyPct(t *testing.T) {
	t.Parallel()
	if got := CalculateSLACredit(89.9, 99.9); got != 50 {
		t.Errorf("CalculateSLACredit(89.9, 99.9) = %d, want 50", got)
	}
}

func TestCalculateSLACredit_HigherTarget_ExtendedRange(t *testing.T) {
	t.Parallel()
	got := CalculateSLACredit(99.91, 99.95)
	if got != 10 {
		t.Errorf("CalculateSLACredit(99.91, 99.95) = %d, want 10 (extended top tier)", got)
	}
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
		if got != tt.expected {
			t.Errorf("CalculateSLACredit(%.2f, %.1f) = %d, want %d",
				tt.uptime, tt.target, got, tt.expected)
		}
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
	if err == nil {
		t.Fatal("expected spending limit error")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T", err)
	}
	if !strings.Contains(le.Message, "$50.00") {
		t.Errorf("message should contain $50.00, got: %s", le.Message)
	}
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
	if err == nil {
		t.Fatal("expected free-tier spending limit error")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T", err)
	}
	// Message should reference budget being reached (no dollar amount for $0 credit).
	if !strings.Contains(le.Message, "budget") && !strings.Contains(le.Message, "compute") {
		t.Errorf("message should reference compute budget, got: %s", le.Message)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range impact.Impacts {
		if imp.Resource == "regions" {
			t.Fatalf("downgrade preview exposed launch-inactive regions impact: %#v", imp)
		}
	}
}
