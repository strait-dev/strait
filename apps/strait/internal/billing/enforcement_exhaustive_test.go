package billing

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
)

// G. Spending Limit Enforcement (10 tests).

func TestSpendingEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tier          string
		spendingLimit int64
		periodSpend   int64
		wantBlock     bool
	}{
		{"free_under_credit", "free", 0, CreditFreeMicrousd - 1, false},
		{"free_at_credit", "free", 0, CreditFreeMicrousd, false},
		{"free_over_credit", "free", 0, CreditFreeMicrousd + 1, true},
		{"starter_no_limit_overage", "starter", -1, CreditStarterMicrousd + 50_000_000, false},
		{"pro_50_limit_under", "pro", 50_000_000, CreditProMicrousd + 49_990_000, false},
		{"pro_50_limit_over", "pro", 50_000_000, CreditProMicrousd + 50_010_000, true},
		{"pro_0_cap_at_credit", "pro", 0, CreditProMicrousd + 1, true},
		{"scale_no_limit", "scale", -1, CreditScaleMicrousd + 200_000_000, false},
		{"enterprise_no_block", "enterprise", -1, 500_000_000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			now := time.Now()
			ps := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			pe := ps.AddDate(0, 1, 0)

			store := &mockBillingStore{
				subscriptions: map[string]*OrgSubscription{
					"org": {
						OrgID:                 "org",
						PlanTier:              tt.tier,
						Status:                "active",
						SpendingLimitMicrousd: tt.spendingLimit,
						LimitAction:           "reject",
						CurrentPeriodStart:    &ps,
						CurrentPeriodEnd:      &pe,
					},
				},
				periodSpendByOrg: map[string]int64{"org": tt.periodSpend},
			}

			enforcer := NewEnforcer(store, rdb, slog.Default())
			err := enforcer.CheckSpendingLimit(context.Background(), "org")
			blocked := err != nil
			if blocked != tt.wantBlock {
				t.Errorf("blocked = %v, want %v (err: %v)", blocked, tt.wantBlock, err)
			}
		})
	}

	t.Run("concurrent_50_goroutines", func(t *testing.T) {
		t.Parallel()
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

		now := time.Now()
		ps := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		pe := ps.AddDate(0, 1, 0)

		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-conc": {
					OrgID:                 "org-conc",
					PlanTier:              "pro",
					Status:                "active",
					SpendingLimitMicrousd: 10_000_000,
					LimitAction:           "reject",
					CurrentPeriodStart:    &ps,
					CurrentPeriodEnd:      &pe,
				},
			},
			periodSpendByOrg: map[string]int64{"org-conc": CreditProMicrousd + 9_990_000},
		}

		enforcer := NewEnforcer(store, rdb, slog.Default())
		var wg conc.WaitGroup
		var errCount atomic.Int64

		for range 50 {
			wg.Go(func() {
				if err := enforcer.CheckSpendingLimit(context.Background(), "org-conc"); err != nil {
					errCount.Add(1)
				}
			})
		}

		wg.Wait()
		// All should pass since we're just under the limit.
		if errCount.Load() != 0 {
			t.Errorf("expected 0 errors, got %d", errCount.Load())
		}
	})
}

// H. Grace Period Enforcement (8 tests).

func TestGracePeriodEnforcement(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-1 * time.Hour)
	exactNow := time.Now().Add(-1 * time.Millisecond) // just expired

	tests := []struct {
		name          string
		tier          string
		paymentStatus string
		graceEnd      *time.Time
		wantBlock     bool
		wantCode      string
	}{
		{"scale_active_grace", "scale", "grace", &future, false, ""},
		{"scale_expired_grace", "scale", "grace", &past, true, "grace_period_expired"},
		{"scale_restricted", "scale", "restricted", nil, true, "payment_restricted"},
		{"pro_active_grace", "pro", "grace", &future, false, ""},
		{"pro_ok_no_check", "pro", "ok", nil, false, ""},
		{"scale_exact_expiry", "scale", "grace", &exactNow, true, "grace_period_expired"},
		{"starter_past_due_grace", "starter", "grace", &future, false, ""},
		{"free_no_payment", "free", "", nil, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			store := &mockBillingStore{}
			if tt.tier != "free" || tt.paymentStatus != "" {
				store.subscriptions = map[string]*OrgSubscription{
					"org": {
						OrgID:          "org",
						PlanTier:       tt.tier,
						Status:         "active",
						PaymentStatus:  tt.paymentStatus,
						GracePeriodEnd: tt.graceEnd,
					},
				}
			}

			enforcer := NewEnforcer(store, rdb, slog.Default())
			err := enforcer.CheckDailyRunLimit(context.Background(), "org")

			if tt.wantBlock {
				if err == nil {
					t.Fatal("expected block, got nil")
				}
				var le *LimitError
				if isLimitError(err, &le) && tt.wantCode != "" {
					if le.Code != tt.wantCode {
						t.Errorf("Code = %q, want %q", le.Code, tt.wantCode)
					}
				}
			} else if err != nil {
				t.Fatalf("expected pass, got: %v", err)
			}
		})
	}
}

// I. Addon Enforcement (12 tests).

func TestAddonEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tier      domain.PlanTier
		addons    []Addon
		checkFn   func(OrgPlanLimits) int
		wantValue int
	}{
		{"pro_1_concurrent_pack", domain.PlanPro, []Addon{{AddonType: AddonConcurrentRuns, Quantity: 1, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, ConcurrentPro + 50},
		{"pro_2_concurrent_packs", domain.PlanPro, []Addon{{AddonType: AddonConcurrentRuns, Quantity: 2, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, ConcurrentPro + 100},
		{"pro_5_member_packs", domain.PlanPro, []Addon{{AddonType: AddonMembers, Quantity: 5, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxMembersPerOrg }, MaxMembersPro + 5},
		{"starter_1_retention_pack", domain.PlanStarter, []Addon{{AddonType: AddonDataRetention, Quantity: 1, Active: true}},
			func(l OrgPlanLimits) int { return l.RetentionDays }, RetentionStarter + 30},
		{"starter_3_retention_capped", domain.PlanStarter, []Addon{{AddonType: AddonDataRetention, Quantity: 3, Active: true}},
			func(l OrgPlanLimits) int { return l.RetentionDays }, 90},
		{"pro_1_cron_pack", domain.PlanPro, []Addon{{AddonType: AddonCronSchedules, Quantity: 1, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxScheduledJobs }, MaxScheduledPro + 25},
		{"pro_2_webhook_packs", domain.PlanPro, []Addon{{AddonType: AddonWebhookEndpoints, Quantity: 2, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxWebhookEndpoints }, 10 + 10},
		{"deactivated_ignored", domain.PlanPro, []Addon{{AddonType: AddonConcurrentRuns, Quantity: 5, Active: false}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, ConcurrentPro},
		{"quantity_0_ignored", domain.PlanPro, []Addon{{AddonType: AddonConcurrentRuns, Quantity: 0, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, ConcurrentPro},
		{"negative_quantity_ignored", domain.PlanPro, []Addon{{AddonType: AddonConcurrentRuns, Quantity: -1, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, ConcurrentPro},
		{"unknown_type_ignored", domain.PlanPro, []Addon{{AddonType: AddonType("nonexistent"), Quantity: 10, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, ConcurrentPro},
		{"enterprise_stays_unlimited", domain.PlanEnterprise, []Addon{{AddonType: AddonConcurrentRuns, Quantity: 10, Active: true}},
			func(l OrgPlanLimits) int { return l.MaxConcurrentRuns }, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := GetPlanLimits(tt.tier)
			result := EffectiveLimits(base, tt.addons)
			got := tt.checkFn(result)
			if got != tt.wantValue {
				t.Errorf("%s: got %d, want %d", tt.name, got, tt.wantValue)
			}
		})
	}
}

// M. Stripe Usage Event Ingestion (6 tests).

func TestStripeUsageEnforcement(t *testing.T) {
	// Not parallel: NewStripeUsageReporter writes to the global stripe.Key,
	// which cannot be safely accessed from concurrent goroutines.

	t.Run("empty_secret_skipped", func(t *testing.T) {
		reporter := NewStripeUsageReporter("", slog.Default())
		err := reporter.IngestComputeUsage(context.Background(), "cust-1", "run-1", 1700)
		if err != nil {
			t.Fatalf("expected nil for empty secret key, got: %v", err)
		}
	})

	t.Run("empty_customer_skipped", func(t *testing.T) {
		reporter := NewStripeUsageReporter("sk_test_key", slog.Default())
		err := reporter.IngestComputeUsage(context.Background(), "", "run-1", 1700)
		if err != nil {
			t.Fatalf("expected nil for empty customer ID, got: %v", err)
		}
	})

	t.Run("nil_logger_safe", func(t *testing.T) {
		reporter := NewStripeUsageReporter("sk_test_key", nil)
		if reporter == nil {
			t.Fatal("expected non-nil reporter with nil logger")
		}
	})

	t.Run("with_metrics_option", func(t *testing.T) {
		reporter := NewStripeUsageReporter("sk_test_key", slog.Default(), WithUsageReporterMetrics(nil))
		if reporter == nil {
			t.Fatal("expected non-nil reporter with metrics option")
		}
	})

	t.Run("constructor_returns_non_nil", func(t *testing.T) {
		reporter := NewStripeUsageReporter("sk_test_key", slog.Default())
		if reporter == nil {
			t.Fatal("expected non-nil reporter")
		}
	})

	t.Run("no_secret_skipped", func(t *testing.T) {
		reporter := NewStripeUsageReporter("", slog.Default())
		err := reporter.IngestComputeUsage(context.Background(), "cust-1", "run-1", 100)
		if err != nil {
			t.Errorf("expected nil for no-secret skip, got: %v", err)
		}
	})
}

// O. Stripe Webhook Handling (8 tests).

func TestStripeWebhookEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("scale_subscription_creates_record", func(t *testing.T) {
		t.Parallel()
		mapping := NewStripeMappingFromOptions(
			WithScalePrices("scale-m", ""),
		)
		store := &mockBillingStore{}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		body := `{"id":"evt-scale","type":"customer.subscription.created","data":{"object":{"id":"sub-1","status":"active","items":{"data":[{"price":{"id":"scale-m"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"00000000-0000-0000-0000-000000000040"}}}}}`
		req := httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if store.lastUpserted == nil {
			t.Fatal("expected subscription to be upserted")
		}
		if store.lastUpserted.PlanTier != "scale" {
			t.Errorf("PlanTier = %q, want scale", store.lastUpserted.PlanTier)
		}
	})

	t.Run("addon_subscription_creates_record", func(t *testing.T) {
		t.Parallel()
		mapping := NewStripeMappingFromOptions(
			WithAddonPrice("addon-cr", AddonConcurrentRuns),
		)
		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
		}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		body := `{"id":"evt-addon","type":"customer.subscription.created","data":{"object":{"id":"addon-sub-1","status":"active","items":{"data":[{"price":{"id":"addon-cr"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"00000000-0000-0000-0000-000000000040"}}}}}`
		req := httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("unknown_product_errors", func(t *testing.T) {
		t.Parallel()
		mapping := NewStripeMappingFromOptions()
		store := &mockBillingStore{}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		body := `{"id":"evt-unknown","type":"customer.subscription.created","data":{"object":{"id":"sub-x","status":"active","items":{"data":[{"price":{"id":"unknown-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"00000000-0000-0000-0000-000000000040"}}}}}`
		req := httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != 500 {
			t.Errorf("expected 500 for unknown product, got %d", w.Code)
		}
	})

	t.Run("duplicate_subscription_idempotent", func(t *testing.T) {
		t.Parallel()
		mapping := NewStripeMappingFromOptions(WithStarterPrices("starter-m", ""))
		store := &mockBillingStore{}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		bodyTpl := `{"id":"evt-dup-%d","type":"customer.subscription.created","data":{"object":{"id":"sub-dup","status":"active","items":{"data":[{"price":{"id":"starter-m"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"00000000-0000-0000-0000-000000000040"}}}}}`

		// Send twice with unique event IDs to avoid replay dedup.
		for i := range 2 {
			body := fmt.Sprintf(bodyTpl, i)
			req := httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(body))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("expected 200, got %d", w.Code)
			}
		}

		if store.upsertCount != 2 {
			t.Errorf("upsert count = %d, want 2 (idempotent upsert)", store.upsertCount)
		}
	})
}
