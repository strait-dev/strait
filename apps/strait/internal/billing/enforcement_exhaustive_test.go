package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
		var wg sync.WaitGroup
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

// M. Polar Event Ingestion (6 tests).

func TestPolarEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("managed_run_correct_payload", func(t *testing.T) {
		t.Parallel()
		var received []polarEvent
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req polarIngestRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			received = append(received, req.Events...)
			w.WriteHeader(200)
		}))
		defer srv.Close()

		ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())
		err := ingester.IngestComputeUsage(context.Background(), "cust-1", "run-1", 1700)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(received) != 1 {
			t.Fatalf("expected 1 event, got %d", len(received))
		}
		if received[0].ExternalCustomerID != "cust-1" {
			t.Errorf("customer ID = %q, want cust-1", received[0].ExternalCustomerID)
		}
		if received[0].Metadata["amount"] != "1700" {
			t.Errorf("amount = %q, want 1700", received[0].Metadata["amount"])
		}
		if received[0].ExternalID != "run-1" {
			t.Errorf("external ID = %q, want run-1", received[0].ExternalID)
		}
	})

	t.Run("http_run_flat_20", func(t *testing.T) {
		t.Parallel()
		var received []polarEvent
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req polarIngestRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			received = append(received, req.Events...)
			w.WriteHeader(200)
		}))
		defer srv.Close()

		ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())
		_ = ingester.IngestComputeUsage(context.Background(), "cust-1", "run-http", HTTPCostPerRunMicrousd)
		if len(received) != 1 {
			t.Fatalf("expected 1 event, got %d", len(received))
		}
		if received[0].Metadata["amount"] != fmt.Sprintf("%d", HTTPCostPerRunMicrousd) {
			t.Errorf("amount = %q, want %d", received[0].Metadata["amount"], HTTPCostPerRunMicrousd)
		}
	})

	t.Run("429_no_crash", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"detail":"rate limited"}`))
		}))
		defer srv.Close()

		ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())
		err := ingester.IngestComputeUsage(context.Background(), "cust-1", "run-1", 100)
		if err == nil {
			t.Error("expected error for 429")
		}
		// No panic -- test passes.
	})

	t.Run("500_no_crash", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"detail":"internal error"}`))
		}))
		defer srv.Close()

		ingester := NewPolarEventIngester(srv.URL, "test-token", slog.Default())
		err := ingester.IngestComputeUsage(context.Background(), "cust-1", "run-1", 100)
		if err == nil {
			t.Error("expected error for 500")
		}
	})

	t.Run("unreachable_no_block", func(t *testing.T) {
		t.Parallel()
		ingester := NewPolarEventIngester("http://127.0.0.1:1", "test-token", slog.Default())
		err := ingester.IngestComputeUsage(context.Background(), "cust-1", "run-1", 100)
		if err == nil {
			t.Error("expected error for unreachable server")
		}
		// Key: the ingestion is fire-and-forget in the executor, so even though
		// this returns an error, runs are NOT blocked.
	})

	t.Run("no_token_skipped", func(t *testing.T) {
		t.Parallel()
		ingester := NewPolarEventIngester("http://example.com", "", slog.Default())
		err := ingester.IngestComputeUsage(context.Background(), "cust-1", "run-1", 100)
		if err != nil {
			t.Errorf("expected nil for no-token skip, got: %v", err)
		}
	})
}

// O. Polar Webhook Handling (8 tests).

func TestPolarWebhookEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("scale_subscription_creates_record", func(t *testing.T) {
		t.Parallel()
		mapping := NewPolarMappingFromOptions(
			WithScaleProducts("scale-m", ""),
		)
		store := &mockBillingStore{}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		body := `{"type":"subscription.created","data":{"id":"sub-1","status":"active","product_id":"scale-m","customer_id":"cust-1","customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"org-1"}}}}`
		req := httptest.NewRequest("POST", "/polar/webhook", strings.NewReader(body))
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
		mapping := NewPolarMappingFromOptions(
			WithAddonProduct("addon-cr", AddonConcurrentRuns),
		)
		store := &mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-1": {OrgID: "org-1", PlanTier: "pro", Status: "active"},
			},
		}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		body := `{"type":"subscription.created","data":{"id":"addon-sub-1","status":"active","product_id":"addon-cr","customer_id":"cust-1","customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"org-1"}}}}`
		req := httptest.NewRequest("POST", "/polar/webhook", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("unknown_product_errors", func(t *testing.T) {
		t.Parallel()
		mapping := NewPolarMappingFromOptions()
		store := &mockBillingStore{}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		body := `{"type":"subscription.created","data":{"id":"sub-x","status":"active","product_id":"unknown-id","customer_id":"cust-1","customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"org-1"}}}}`
		req := httptest.NewRequest("POST", "/polar/webhook", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != 500 {
			t.Errorf("expected 500 for unknown product, got %d", w.Code)
		}
	})

	t.Run("duplicate_subscription_idempotent", func(t *testing.T) {
		t.Parallel()
		mapping := NewPolarMappingFromOptions(WithStarterProducts("starter-m", ""))
		store := &mockBillingStore{}
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

		body := `{"type":"subscription.created","data":{"id":"sub-dup","status":"active","product_id":"starter-m","customer_id":"cust-1","customer":{"id":"cust-1","email":"test@example.com","metadata":{"org_id":"org-1"}}}}`

		// Send twice.
		for range 2 {
			req := httptest.NewRequest("POST", "/polar/webhook", strings.NewReader(body))
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
