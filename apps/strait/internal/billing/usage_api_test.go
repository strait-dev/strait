package billing

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

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
		aiModelCallCounts: map[string]int64{
			"org_test": 7,
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
	if resp.Usage.RunsToday.Limit != 5000 {
		t.Errorf("runs limit = %d, want 5000", resp.Usage.RunsToday.Limit)
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
	if resp.Usage.AIModelCalls.Used != 7 {
		t.Errorf("ai model calls used = %d, want 7", resp.Usage.AIModelCalls.Used)
	}
	if resp.Usage.AIAssistantMessages != resp.Usage.AIModelCalls {
		t.Errorf("deprecated ai assistant field = %+v, want %+v", resp.Usage.AIAssistantMessages, resp.Usage.AIModelCalls)
	}
	assertFloatApprox(t, resp.Usage.AIModelCalls.Percent, 35)
	assertFloatApprox(t, resp.Usage.ConcurrentRuns.Percent, 60)
	assertFloatApprox(t, resp.Usage.Members.Percent, 66.6666666667)
	if resp.Usage.RetentionDays != 1 {
		t.Errorf("retention = %d, want 1", resp.Usage.RetentionDays)
	}
}

func TestUsageService_GetCurrentUsage_EnterpriseAIModelCallsRemainUnlimited(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_enterprise": {
				OrgID:    "org_enterprise",
				PlanTier: "enterprise",
				Status:   "active",
			},
		},
		aiModelCallCounts: map[string]int64{
			"org_enterprise": 42,
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_enterprise")
	if err != nil {
		t.Fatal(err)
	}

	if resp.Plan != "enterprise" {
		t.Fatalf("plan = %q, want enterprise", resp.Plan)
	}
	if resp.Usage.AIModelCalls.Limit != -1 {
		t.Fatalf("ai model calls limit = %d, want -1", resp.Usage.AIModelCalls.Limit)
	}
	if resp.Usage.AIModelCalls.Used != 42 {
		t.Fatalf("ai model calls used = %d, want 42", resp.Usage.AIModelCalls.Used)
	}
	if resp.Usage.AIModelCalls.Percent != 0 {
		t.Fatalf("ai model calls percent = %f, want 0", resp.Usage.AIModelCalls.Percent)
	}
}

func TestUsageService_AlertsAt80Percent(t *testing.T) {
	t.Parallel()

	svc, enforcer := newUsageServiceTest(t, &mockBillingStore{})

	// Simulate 4100 runs (82% of 5000 free limit)
	ctx := context.Background()
	for range 4100 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_alert")
	}

	resp, err := svc.GetCurrentUsage(ctx, "org_alert")
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Alerts) == 0 {
		t.Fatal("expected alerts at 82% usage")
	}
	if resp.Alerts[0].Dimension != "runs_today" {
		t.Errorf("alert dimension = %q, want runs_today", resp.Alerts[0].Dimension)
	}
}

func TestUsageService_GetUsageHistory_IncludesAIUsage(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{
				PeriodDate:       time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				RunsCount:        4,
				ComputeCostMicro: 2_500_000,
				AITokensTotal:    1200,
				AICostMicro:      1_250_000,
			},
			{
				PeriodDate:       time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
				RunsCount:        2,
				ComputeCostMicro: 500_000,
				AITokensTotal:    300,
				AICostMicro:      400_000,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	history, err := svc.GetUsageHistory(context.Background(), "org-ai", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].AITokens != 1200 {
		t.Fatalf("day 1 ai tokens = %d, want 1200", history[0].AITokens)
	}
	if history[0].AICostMicro != 1_250_000 {
		t.Fatalf("day 1 ai cost = %d, want 1250000", history[0].AICostMicro)
	}
}

func TestUsageService_GetUsageForecast_UsesAICost(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{
				PeriodDate:       now.AddDate(0, 0, -1),
				RunsCount:        10,
				ComputeCostMicro: 2_000_000,
				AICostMicro:      1_000_000,
			},
			{
				PeriodDate:       now,
				RunsCount:        20,
				ComputeCostMicro: 4_000_000,
				AICostMicro:      2_000_000,
			},
		},
	}
	svc, _ := newUsageServiceTest(t, store)

	forecast, err := svc.GetUsageForecast(context.Background(), "org-forecast")
	if err != nil {
		t.Fatal(err)
	}

	assertFloatApprox(t, forecast.ProjectedMonthlyComputeUsd, 90)
	assertFloatApprox(t, forecast.ProjectedMonthlyAICostUsd, 45)
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
	assertFloatApprox(t, resp.OverageSpendUsd, 1.5)
	assertFloatApprox(t, resp.IncludedCreditUsd, 1)
}

func TestUsageService_GetSpendingLimit_FreeTierWithSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org_free": {OrgID: "org_free", PlanTier: "free", Status: "active", SpendingLimitMicrousd: -1},
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
	assertFloatApprox(t, resp.OverageSpendUsd, 0.25)
	assertFloatApprox(t, resp.IncludedCreditUsd, 1)
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
		{"moderate", 200000, 10000000, "starter"},
		{"high", 1000000, 30000000, "pro"},
		{"very_high", 5000000, 60000000, "enterprise"},
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
			"org_starter": 25_000_000, // exceeds starter credit of 19,990,000
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
			break
		}
	}
	if !foundOverageAlert {
		t.Error("expected overage alert for paid plan with spend exceeding credit")
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

func TestUsageService_GetCurrentUsage_FreeTierAtIncludedCredit(t *testing.T) {
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
	if resp.IncludedCreditMicro != CreditFreeMicrousd {
		t.Fatalf("included credit = %d, want %d", resp.IncludedCreditMicro, CreditFreeMicrousd)
	}
	if resp.PeriodSpendMicro != CreditFreeMicrousd {
		t.Fatalf("period spend = %d, want %d", resp.PeriodSpendMicro, CreditFreeMicrousd)
	}
	if resp.OverageMicro != 0 {
		t.Fatalf("overage = %d, want 0", resp.OverageMicro)
	}
	assertFloatApprox(t, resp.Usage.ComputeCredit.Percent, 100)

	for _, alert := range resp.Alerts {
		if alert.Dimension == "overage" {
			t.Fatal("free plan at included credit should not have overage alert")
		}
	}
}

func TestUsageService_GetCurrentUsage_FreeTierOverIncludedCredit(t *testing.T) {
	t.Parallel()

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

	if resp.OverageMicro != 250_000 {
		t.Fatalf("overage = %d, want 250000", resp.OverageMicro)
	}
	assertFloatApprox(t, resp.Usage.ComputeCredit.Percent, 125)

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
