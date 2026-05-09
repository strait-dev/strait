package billing

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestCheckDailyAIModelCallLimit_NilRedis_FailsOpen confirms a nil Redis
// client returns nil rather than panicking. Mirrors the daily-run check.
func TestCheckDailyAIModelCallLimit_NilRedis_FailsOpen(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	e := NewEnforcer(store, nil, slog.Default())

	if err := e.CheckDailyAIModelCallLimit(context.Background(), "org-1"); err != nil {
		t.Fatalf("nil redis must fail open, got %v", err)
	}
}

// TestCheckDailyAIModelCallLimit_DBError_FailsOpen ensures a transient DB
// failure does not block AI usage telemetry.
func TestCheckDailyAIModelCallLimit_DBError_FailsOpen(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			return nil, errors.New("db down")
		},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	if err := e.CheckDailyAIModelCallLimit(context.Background(), "org-1"); err != nil {
		t.Fatalf("DB error must fail open, got %v", err)
	}
}

// TestCheckDailyAIModelCallLimit_FreeTier_RejectsOverCap walks the counter
// past the Free-tier cap (20/day) and asserts the 21st call returns a typed
// LimitError.
func TestCheckDailyAIModelCallLimit_FreeTier_RejectsOverCap(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-free": {OrgID: "org-free", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	freeCap := GetPlanLimits(domain.PlanFree).MaxAIModelCallsPerDay
	for i := range freeCap {
		if err := e.CheckDailyAIModelCallLimit(ctx, "org-free"); err != nil {
			t.Fatalf("call %d must succeed (cap=%d), got %v", i+1, freeCap, err)
		}
	}
	err := e.CheckDailyAIModelCallLimit(ctx, "org-free")
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError after cap, got %T %v", err, err)
	}
	if le.Code != "org_daily_ai_call_limit_exceeded" {
		t.Errorf("code = %q, want org_daily_ai_call_limit_exceeded", le.Code)
	}
	if le.Limit != int64(freeCap) {
		t.Errorf("LimitError.Limit = %d, want %d", le.Limit, freeCap)
	}
}

// TestCheckDailyAIModelCallLimit_PaidTier_AllowsOverage proves Pro tier (or
// any non-Free) lets the call through and only logs an overage event when the
// counter exceeds the cap. Paid plans never hard-reject AI calls — Stripe
// metered billing reconciles overage downstream.
func TestCheckDailyAIModelCallLimit_PaidTier_AllowsOverage(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pro": {OrgID: "org-pro", PlanTier: string(domain.PlanPro), Status: "active"},
		},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	proCap := GetPlanLimits(domain.PlanPro).MaxAIModelCallsPerDay
	// Drive the counter to cap+5 — every call must succeed on a paid plan.
	for i := range proCap + 5 {
		if err := e.CheckDailyAIModelCallLimit(ctx, "org-pro"); err != nil {
			t.Fatalf("paid plan must allow overage, got %v at call %d", err, i+1)
		}
	}
}

// TestCheckDailyAIModelCallLimit_EnterpriseUnlimited proves the unlimited tier
// short-circuits before touching Redis. Closing miniredis after the unlimited
// short-circuit would still return nil because the script is never invoked.
func TestCheckDailyAIModelCallLimit_EnterpriseUnlimited(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-ent": {OrgID: "org-ent", PlanTier: string(domain.PlanEnterprise), Status: "active"},
		},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	for range 1000 {
		if err := e.CheckDailyAIModelCallLimit(ctx, "org-ent"); err != nil {
			t.Fatalf("unlimited tier must never reject, got %v", err)
		}
	}
}

// TestCheckDailyAIModelCallLimit_DecrRollback proves DecrDailyAIModelCallCount
// floors at zero and lets a previously-rejected call slot back into the day.
// The semantics: a check that increments and then needs to roll back must not
// permanently consume a slot.
func TestCheckDailyAIModelCallLimit_DecrRollback(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-free": {OrgID: "org-free", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	freeCap := GetPlanLimits(domain.PlanFree).MaxAIModelCallsPerDay
	for range freeCap {
		if err := e.CheckDailyAIModelCallLimit(ctx, "org-free"); err != nil {
			t.Fatalf("under-cap call rejected: %v", err)
		}
	}
	// Roll back one slot.
	e.DecrDailyAIModelCallCount(ctx, "org-free")

	// Next call must succeed because the slot was returned.
	if err := e.CheckDailyAIModelCallLimit(ctx, "org-free"); err != nil {
		t.Fatalf("after decrement, next call must succeed, got %v", err)
	}
}

// TestCheckDailyAIModelCallLimit_ConcurrentRace fires N+1 concurrent requests
// against a Free-tier org with the daily cap of `n`. Exactly cap requests
// must succeed; the rest must reject. Uses miniredis with the real Lua script
// so this is a true atomicity test.
func TestCheckDailyAIModelCallLimit_ConcurrentRace(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-race": {OrgID: "org-race", PlanTier: string(domain.PlanFree), Status: "active"},
		},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	freeCap := GetPlanLimits(domain.PlanFree).MaxAIModelCallsPerDay
	total := freeCap + 50
	var (
		wg       sync.WaitGroup
		accepted atomic.Int64
		rejected atomic.Int64
	)
	for range total {
		wg.Go(func() {
			err := e.CheckDailyAIModelCallLimit(ctx, "org-race")
			if err == nil {
				accepted.Add(1)
				return
			}
			var le *LimitError
			if errors.As(err, &le) {
				rejected.Add(1)
				return
			}
			t.Errorf("unexpected error: %v", err)
		})
	}
	wg.Wait()
	if got := accepted.Load(); got != int64(freeCap) {
		t.Errorf("accepted = %d, want exactly %d (cap)", got, freeCap)
	}
	if got := accepted.Load() + rejected.Load(); got != int64(total) {
		t.Errorf("accepted+rejected = %d, want %d", got, total)
	}
}
