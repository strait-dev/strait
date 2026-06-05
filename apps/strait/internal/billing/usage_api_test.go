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

	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("got %f, want %f", got, want)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	if resp.Plan != "free" {
		t.Errorf("plan = %q, want free", resp.Plan)
	}
	freeLimits := GetPlanLimits(domain.PlanFree)
	if resp.Usage.MonthlyRuns.Limit != int64(freeLimits.MaxRunsPerMonth) {
		t.Errorf("monthly runs limit = %d, want %d", resp.Usage.MonthlyRuns.Limit, freeLimits.MaxRunsPerMonth)
	}
	if resp.Usage.RunsToday != resp.Usage.MonthlyRuns {
		t.Errorf("legacy runs_today = %+v, want monthly_runs alias %+v", resp.Usage.RunsToday, resp.Usage.MonthlyRuns)
	}
	if resp.Usage.Projects.Used != 1 {
		t.Errorf("projects used = %d, want 1", resp.Usage.Projects.Used)
	}
	if resp.Usage.Members.Used != 2 {
		t.Errorf("members used = %d, want 2", resp.Usage.Members.Used)
	}
	if resp.Usage.ConcurrentRuns.Used != 3 {
		t.Errorf("concurrent runs used = %d, want 3", resp.Usage.ConcurrentRuns.Used)
	}
	// ConcurrentRuns: 3 used / ConcurrentFree limit.
	expectedConcPct := safePercent(3, int64(freeLimits.MaxConcurrentRuns))
	assertFloatApprox(t, resp.Usage.ConcurrentRuns.Percent, expectedConcPct)
	// Members: 2 used / MaxMembersFree limit.
	expectedMemberPct := safePercent(2, int64(freeLimits.MaxMembersPerOrg))
	assertFloatApprox(t, resp.Usage.Members.Percent, expectedMemberPct)
	if resp.Usage.RetentionDays != RetentionFree {
		t.Errorf("retention = %d, want %d", resp.Usage.RetentionDays, RetentionFree)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.ActiveAddons) != 1 {
		t.Fatalf("ActiveAddons len = %d, want 1: %+v", len(resp.ActiveAddons), resp.ActiveAddons)
	}
	if resp.ActiveAddons[0].Type != string(AddonConcurrency100) {
		t.Fatalf("ActiveAddons[0].Type = %q, want %q", resp.ActiveAddons[0].Type, AddonConcurrency100)
	}
	if resp.ActiveAddons[0].Quantity != 2 {
		t.Fatalf("ActiveAddons[0].Quantity = %d, want 2", resp.ActiveAddons[0].Quantity)
	}
}

func TestUsageService_NoAlertsForLowMonthlyRuns(t *testing.T) {
	t.Parallel()

	svc, enforcer := newUsageServiceTest(t, &mockBillingStore{})

	ctx := context.Background()
	for range 100 {
		_ = enforcer.CheckMonthlyRunLimit(ctx, "org_alert")
	}

	resp, err := svc.GetCurrentUsage(ctx, "org_alert")
	if err != nil {
		t.Fatal(err)
	}

	for _, alert := range resp.Alerts {
		if alert.Dimension == "monthly_runs" {
			t.Error("unexpected monthly_runs alert below threshold")
		}
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
	if err != nil {
		t.Fatal(err)
	}

	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].RunsCount != 4 {
		t.Fatalf("day 1 runs = %d, want 4", history[0].RunsCount)
	}
	if history[0].SpendMicro != 2_500_000 {
		t.Fatalf("day 1 spend = %d, want 2500000", history[0].SpendMicro)
	}
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
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	if resp.PlanTier != "free" {
		t.Fatalf("plan tier = %q, want free", resp.PlanTier)
	}
	if resp.LimitAction != "reject" {
		t.Fatalf("limit action = %q, want reject", resp.LimitAction)
	}
	if !resp.IsHardCapped {
		t.Fatal("expected free tier response to be hard capped")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	if resp.PlanTier != "free" {
		t.Fatalf("plan tier = %q, want free", resp.PlanTier)
	}
	if resp.LimitAction != "reject" {
		t.Fatalf("limit action = %q, want reject", resp.LimitAction)
	}
	if !resp.IsHardCapped {
		t.Fatal("expected free tier response to be hard capped")
	}
	assertFloatApprox(t, resp.CurrentSpendUsd, 1.25)
	assertFloatApprox(t, resp.OverageSpendUsd, 1.25)
}

func TestUsageService_SetSpendingLimit_FreeTierWithoutSubscription(t *testing.T) {
	t.Parallel()

	svc, _ := newUsageServiceTest(t, &mockBillingStore{})

	err := svc.SetSpendingLimit(context.Background(), "org_free", 5_000_000, "notify")
	if err == nil {
		t.Fatal("expected free tier spending limit update to fail")
	}
	if err.Error() != "spending limits are not available on the Free plan" {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected negative spending limit to be rejected")
	}
	if err.Error() != "spending limit must be non-negative" {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected large negative spending limit to be rejected")
	}
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
	if err != nil {
		t.Fatalf("zero spending limit should be allowed (hard cap): %v", err)
	}
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
	if err != nil {
		t.Fatalf("valid positive spending limit should be allowed: %v", err)
	}
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
	if err != nil {
		t.Fatalf("raising cap above current spend should resume paused jobs: %v", err)
	}
	if store.unpausedOrgID != "org-1" {
		t.Fatalf("unpaused org = %q, want org-1", store.unpausedOrgID)
	}
	if store.unpausedReason != "quota_exceeded" {
		t.Fatalf("unpaused reason = %q, want quota_exceeded", store.unpausedReason)
	}
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
	if err != nil {
		t.Fatalf("notify cap should resume paused jobs: %v", err)
	}
	if store.unpausedOrgID != "org-1" {
		t.Fatalf("unpaused org = %q, want org-1", store.unpausedOrgID)
	}
	if store.unpausedReason != "quota_exceeded" {
		t.Fatalf("unpaused reason = %q, want quota_exceeded", store.unpausedReason)
	}
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
	if err != nil {
		t.Fatalf("still-blocking cap update should succeed: %v", err)
	}
	if store.unpausedOrgID != "" {
		t.Fatalf("unexpected unpause for still-blocking cap: org=%q reason=%q", store.unpausedOrgID, store.unpausedReason)
	}
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
	if err != nil {
		t.Fatalf("disable overage: %v", err)
	}
	if store.lastOverageDisabledOrg != "org-1" {
		t.Fatalf("overage flag org = %q, want org-1", store.lastOverageDisabledOrg)
	}
	if !store.lastOverageDisabled {
		t.Fatal("expected overage_disabled=true")
	}
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
	if err != nil {
		t.Fatalf("enable overage: %v", err)
	}
	if store.lastOverageDisabled {
		t.Fatal("expected overage_disabled=false")
	}
	if store.unpausedOrgID != "org-1" || store.unpausedReason != "quota_exceeded" {
		t.Fatalf("quota unpause = org %q reason %q, want org-1 quota_exceeded", store.unpausedOrgID, store.unpausedReason)
	}
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
	if err == nil {
		t.Fatal("expected free overage enablement without payment method to fail")
	}
	if err.Error() != "free overage requires a payment method on file" {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err != nil {
		t.Fatalf("free overage enablement with payment method should pass: %v", err)
	}
	if store.lastOverageDisabled {
		t.Fatal("expected overage_disabled=false")
	}
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
	if err == nil {
		t.Fatal("expected invalid action to be rejected")
	}
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
			if got != tt.want {
				t.Errorf("recommendPlan(%d, %d) = %q, want %q", tt.runs, tt.compute, got, tt.want)
			}
		})
	}
}

func TestSafePercent(t *testing.T) {
	t.Parallel()

	if got := safePercent(50, 100); got != 50.0 {
		t.Errorf("safePercent(50, 100) = %f, want 50.0", got)
	}
	if got := safePercent(0, 0); got != 0.0 {
		t.Errorf("safePercent(0, 0) = %f, want 0.0", got)
	}
	if got := safePercent(100, -1); got != 0.0 {
		t.Errorf("safePercent(100, -1) = %f, want 0.0", got)
	}
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
			if got != tt.wantOverage {
				t.Errorf("overage = %d, want %d", got, tt.wantOverage)
			}
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
	if err != nil {
		t.Fatal(err)
	}

	if resp.OverageMicro <= 0 {
		t.Fatalf("expected positive overage, got %d", resp.OverageMicro)
	}

	var foundOverageAlert bool
	for _, alert := range resp.Alerts {
		if alert.Dimension == "overage" {
			foundOverageAlert = true
			if strings.Contains(alert.Message, "included credit") {
				t.Fatal("overage alert must not use compute credit language")
			}
			if !strings.Contains(alert.Message, "included run allowance") {
				t.Fatal("overage alert should describe the included run allowance")
			}
			break
		}
	}
	if !foundOverageAlert {
		t.Error("expected overage alert for paid plan with spend beyond allowance")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	for _, alert := range resp.Alerts {
		if alert.Dimension == "overage" {
			t.Error("free plan should not have overage alert")
		}
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
	if err != nil {
		t.Fatal(err)
	}

	if resp.Plan != "free" {
		t.Fatalf("plan = %q, want free", resp.Plan)
	}
	if resp.PeriodSpendMicro != CreditFreeMicrousd {
		t.Fatalf("period spend = %d, want %d", resp.PeriodSpendMicro, CreditFreeMicrousd)
	}
	if resp.OverageMicro != CreditFreeMicrousd {
		t.Fatalf("overage = %d, want %d", resp.OverageMicro, CreditFreeMicrousd)
	}

	for _, alert := range resp.Alerts {
		if alert.Dimension == "overage" {
			t.Fatal("free plan should not emit paid-plan overage alert")
		}
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
	if err != nil {
		t.Fatal(err)
	}

	for _, alert := range resp.Alerts {
		if alert.Dimension == "overage" {
			t.Fatal("free plan should not emit paid-plan overage alert")
		}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.MonthlyUsageEmail != false {
		t.Errorf("MonthlyUsageEmail = %v, want false", resp.MonthlyUsageEmail)
	}
}

func TestUsageService_GetEmailPreferences_NotFound(t *testing.T) {
	t.Parallel()

	svc, _ := newUsageServiceTest(t, &mockBillingStore{})

	resp, err := svc.GetEmailPreferences(context.Background(), "org-missing")
	if err != nil {
		t.Fatal(err)
	}
	if resp.MonthlyUsageEmail != true {
		t.Errorf("MonthlyUsageEmail = %v, want true (default)", resp.MonthlyUsageEmail)
	}
}

func TestUsageService_UpdateEmailPreferences(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	svc, _ := newUsageServiceTest(t, store)

	err := svc.UpdateEmailPreferences(context.Background(), "org-update", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStddev_Identical(t *testing.T) {
	t.Parallel()
	if got := stddev([]float64{5, 5, 5, 5}); got != 0 {
		t.Errorf("stddev([5,5,5,5]) = %f, want 0", got)
	}
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
					if a.Dimension == "monthly_runs" {
						t.Errorf("expected no alert for monthly_runs at %.1f%%, got %+v", tt.percent, a)
					}
				}
			} else {
				var found bool
				for _, a := range alerts {
					if a.Dimension == "monthly_runs" {
						found = true
						if a.Severity != tt.wantSev {
							t.Errorf("severity = %q, want %q", a.Severity, tt.wantSev)
						}
						if a.Type != tt.wantType {
							t.Errorf("type = %q, want %q", a.Type, tt.wantType)
						}
					}
				}
				if !found {
					t.Errorf("expected alert for monthly_runs at %.1f%%", tt.percent)
				}
			}
		})
	}
}

func TestUsageService_SetAnomalyConfig_Valid(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetAnomalyConfig(context.Background(), "org-1", 2.0, 8.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUsageService_SetAnomalyConfig_WarningTooLow(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetAnomalyConfig(context.Background(), "org-1", 1.0, 5.0)
	if err == nil {
		t.Fatal("expected error for warning <= 1.0")
	}
}

func TestUsageService_SetAnomalyConfig_CriticalBelowWarning(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetAnomalyConfig(context.Background(), "org-1", 3.0, 3.0)
	if err == nil {
		t.Fatal("expected error for critical <= warning")
	}
}

func TestUsageService_SetProjectBudget_Valid(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetProjectBudget(context.Background(), "proj-1", 5_000_000, "reject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUsageService_SetProjectBudget_AcceptsBlockAction(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	svc, _ := newUsageServiceTest(t, store)

	if err := svc.SetProjectBudget(context.Background(), "proj-1", 5_000_000, "block"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUsageService_SetProjectBudget_InvalidAction(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetProjectBudget(context.Background(), "proj-1", 5_000_000, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestUsageService_SetProjectBudget_NegativeNormalized(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	err := svc.SetProjectBudget(context.Background(), "proj-1", -5, "notify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUsageService_GetProjectBudget_DefaultNoBudget(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	resp, err := svc.GetProjectBudget(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want proj-1", resp.ProjectID)
	}
	if resp.MonthlyBudgetMicro != -1 {
		t.Errorf("MonthlyBudgetMicro = %d, want -1", resp.MonthlyBudgetMicro)
	}
	if resp.PercentUsed != 0 {
		t.Errorf("PercentUsed = %f, want 0 (no budget)", resp.PercentUsed)
	}
}

func TestUsageService_GetAnomalyConfig_DefaultsForMissingSubscription(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	resp, err := svc.GetAnomalyConfig(context.Background(), "org-none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WarningThreshold != spikeWarning {
		t.Errorf("WarningThreshold = %f, want %f", resp.WarningThreshold, spikeWarning)
	}
	if resp.CriticalThreshold != spikeCritical {
		t.Errorf("CriticalThreshold = %f, want %f", resp.CriticalThreshold, spikeCritical)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WarningThreshold != spikeWarning {
		t.Errorf("WarningThreshold = %f, want %f (default for zero)", resp.WarningThreshold, spikeWarning)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WarningThreshold != 2.5 {
		t.Errorf("WarningThreshold = %f, want 2.5", resp.WarningThreshold)
	}
	if resp.CriticalThreshold != 8.0 {
		t.Errorf("CriticalThreshold = %f, want 8.0", resp.CriticalThreshold)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.PaymentStatus != "past_due" {
		t.Errorf("PaymentStatus = %q, want past_due", resp.PaymentStatus)
	}
	if resp.GracePeriodEnd == nil {
		t.Fatal("expected non-nil GracePeriodEnd")
	}
	if *resp.GracePeriodEnd == "" {
		t.Error("expected non-empty GracePeriodEnd")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if resp.OverageMicro != CreditStarterMicrousd {
		t.Errorf("overage should equal total spend in orchestration-only mode, got %d want %d", resp.OverageMicro, CreditStarterMicrousd)
	}
}

func TestUsageService_GetUsageForecast_ZeroHistory(t *testing.T) {
	t.Parallel()
	svc, _ := newUsageServiceTest(t, &mockBillingStore{})
	forecast, err := svc.GetUsageForecast(context.Background(), "org-empty")
	if err != nil {
		t.Fatal(err)
	}
	if forecast.ProjectedMonthlyRuns != 0 {
		t.Errorf("ProjectedMonthlyRuns = %d, want 0", forecast.ProjectedMonthlyRuns)
	}
	if forecast.ProjectedMonthlySpendUsd != 0 {
		t.Errorf("ProjectedMonthlySpendUsd = %f, want 0", forecast.ProjectedMonthlySpendUsd)
	}
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
	if err := enforcer.rdb.Set(context.Background(), monthlyRunKey("org-monthly", now), "49980", time.Hour).Err(); err != nil {
		t.Fatal(err)
	}

	forecast, err := svc.GetUsageForecast(context.Background(), "org-monthly")
	if err != nil {
		t.Fatal(err)
	}
	if forecast.DaysUntilLimit != 2 {
		t.Errorf("DaysUntilLimit = %d, want 2 based on monthly run allowance", forecast.DaysUntilLimit)
	}
}
