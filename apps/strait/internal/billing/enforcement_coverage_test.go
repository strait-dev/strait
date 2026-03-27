package billing

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------.
// CheckDailyRunLimit -- remaining branches
// ---------------------------------------------------------------------------.

// TestCheckDailyRunLimit_NilRedis_FailsOpen verifies that a nil Redis client
// causes CheckDailyRunLimit to return nil (fail open) rather than panic.
func TestCheckDailyRunLimit_NilRedis_FailsOpen(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())

	if err := enforcer.CheckDailyRunLimit(context.Background(), "org-1"); err != nil {
		t.Fatalf("expected nil with nil redis, got %v", err)
	}
}

// TestCheckDailyRunLimit_RedisError_FailsOpen verifies that a Redis connectivity
// error causes the check to fail open (return nil) so as not to block runs.
func TestCheckDailyRunLimit_RedisError_FailsOpen(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	// Close miniredis to simulate Redis being unavailable.
	mr.Close()

	err := enforcer.CheckDailyRunLimit(context.Background(), "org-redis-err")
	if err != nil {
		t.Fatalf("expected fail-open on Redis error, got %v", err)
	}
}

// TestCheckDailyRunLimit_ExactBoundary_FreeTier verifies that the Nth run
// (exactly at the limit) succeeds but the (N+1)th is rejected.
func TestCheckDailyRunLimit_ExactBoundary_FreeTier(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	limits := GetPlanLimits(domain.PlanFree)

	// Run exactly up to the limit; last one at the boundary should succeed.
	for i := range limits.MaxRunsPerDay {
		if err := enforcer.CheckDailyRunLimit(ctx, "org-boundary"); err != nil {
			t.Fatalf("unexpected error at run %d: %v", i+1, err)
		}
	}

	// The very next run should be rejected.
	err := enforcer.CheckDailyRunLimit(ctx, "org-boundary")
	if err == nil {
		t.Fatal("expected rejection at limit+1")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.CurrentUsage != limits.MaxRunsPerDay {
		t.Errorf("CurrentUsage = %d, want %d", le.CurrentUsage, limits.MaxRunsPerDay)
	}
}

// TestCheckDailyRunLimit_DBError_FailsOpen verifies that when the billing
// store returns an error for GetOrgSubscription, the daily limit check fails
// open so that runs are not blocked by transient DB issues.
func TestCheckDailyRunLimit_DBError_FailsOpen(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	dbErr := errors.New("connection refused")
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			return nil, dbErr
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckDailyRunLimit(context.Background(), "org-db-err")
	if err != nil {
		t.Fatalf("expected fail-open on DB error, got %v", err)
	}
}

// TestCheckDailyRunLimit_EnforcementModeDisabled verifies that when the
// org's enforcement mode is "disabled", no limit check is enforced.
func TestCheckDailyRunLimit_EnforcementModeDisabled(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-disabled": {
				OrgID:           "org-disabled",
				PlanTier:        string(domain.PlanFree),
				Status:          "active",
				EnforcementMode: "disabled",
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	limits := GetPlanLimits(domain.PlanFree)

	// Burn through the limit.
	for i := range limits.MaxRunsPerDay + 10 {
		if err := enforcer.CheckDailyRunLimit(ctx, "org-disabled"); err != nil {
			t.Fatalf("expected no enforcement (disabled mode), got error at run %d: %v", i+1, err)
		}
	}
}

// TestCheckDailyRunLimit_EnforcementModeWarn verifies that warn mode does not
// reject, even when the limit is exceeded.
func TestCheckDailyRunLimit_EnforcementModeWarn(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-warn": {
				OrgID:           "org-warn",
				PlanTier:        string(domain.PlanFree),
				Status:          "active",
				EnforcementMode: "warn",
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	limits := GetPlanLimits(domain.PlanFree)

	for i := range limits.MaxRunsPerDay + 5 {
		if err := enforcer.CheckDailyRunLimit(ctx, "org-warn"); err != nil {
			t.Fatalf("expected no rejection in warn mode, got error at run %d: %v", i+1, err)
		}
	}
}

// TestCheckDailyRunLimit_PaymentRestricted verifies that an org with
// "restricted" payment status is blocked from running.
func TestCheckDailyRunLimit_PaymentRestricted(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-restricted": {
				OrgID:         "org-restricted",
				PlanTier:      "starter",
				Status:        "active",
				PaymentStatus: "restricted",
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	err := enforcer.CheckDailyRunLimit(context.Background(), "org-restricted")
	if err == nil {
		t.Fatal("expected payment restriction error")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T", err)
	}
	if le.Code != "payment_restricted" {
		t.Errorf("Code = %q, want payment_restricted", le.Code)
	}
}

// ---------------------------------------------------------------------------.
// DecrDailyRunCount -- decrement paths and error handling
// ---------------------------------------------------------------------------.

// TestDecrDailyRunCount_EmptyOrgID verifies that decrementing with an empty
// org ID is a no-op (does not panic or error).
func TestDecrDailyRunCount_EmptyOrgID(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	// Should not panic.
	enforcer.DecrDailyRunCount(context.Background(), "")
}

// TestDecrDailyRunCount_NilRedis verifies that decrementing with nil Redis
// does not panic.
func TestDecrDailyRunCount_NilRedis(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())

	// Should not panic.
	enforcer.DecrDailyRunCount(context.Background(), "org-1")
}

// TestDecrDailyRunCount_RedisError_DoesNotPanic verifies that a Redis error
// during decrement is handled gracefully (logged, not panicked).
func TestDecrDailyRunCount_RedisError_DoesNotPanic(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	mr.Close()

	// Should not panic even though Redis is down.
	enforcer.DecrDailyRunCount(context.Background(), "org-redis-err")
}

// TestDecrDailyRunCount_FloorsAtZero verifies that decrementing when the
// counter is already at zero does not produce a negative value.
func TestDecrDailyRunCount_FloorsAtZero(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()

	// Increment once, then decrement twice. Counter should not go negative.
	if err := enforcer.CheckDailyRunLimit(ctx, "org-floor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	enforcer.DecrDailyRunCount(ctx, "org-floor")
	enforcer.DecrDailyRunCount(ctx, "org-floor")

	// Counter should still allow runs (zero or positive).
	if err := enforcer.CheckDailyRunLimit(ctx, "org-floor"); err != nil {
		t.Fatalf("counter went negative; expected pass, got: %v", err)
	}
}

// TestDecrDailyRunCount_RollbackAllowsOneMore verifies the full decrement
// round-trip: exhaust the limit, decrement, then verify one more run is allowed.
func TestDecrDailyRunCount_RollbackAllowsOneMore(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()
	limits := GetPlanLimits(domain.PlanFree)

	for i := range limits.MaxRunsPerDay {
		if err := enforcer.CheckDailyRunLimit(ctx, "org-rollback2"); err != nil {
			t.Fatalf("unexpected error at run %d: %v", i+1, err)
		}
	}

	// Verify we are at the limit.
	if err := enforcer.CheckDailyRunLimit(ctx, "org-rollback2"); err == nil {
		t.Fatal("expected limit error")
	}

	// Decrement once (simulating a failed run rollback).
	enforcer.DecrDailyRunCount(ctx, "org-rollback2")

	// Now one more run should be allowed.
	if err := enforcer.CheckDailyRunLimit(ctx, "org-rollback2"); err != nil {
		t.Fatalf("expected pass after decrement, got: %v", err)
	}

	// And the next one should be rejected again.
	if err := enforcer.CheckDailyRunLimit(ctx, "org-rollback2"); err == nil {
		t.Fatal("expected rejection after single rollback slot consumed")
	}
}

// ---------------------------------------------------------------------------.
// WithMetrics -- functional option
// ---------------------------------------------------------------------------.

// TestWithMetrics_NilMetrics verifies that passing nil metrics does not panic.
func TestWithMetrics_NilMetrics(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	enforcer := NewEnforcer(store, rdb, slog.Default(), WithMetrics(nil))
	if enforcer.metrics != nil {
		t.Fatal("expected nil metrics after WithMetrics(nil)")
	}
}

// TestWithMetrics_SetsMetrics verifies that WithMetrics correctly sets the
// metrics field on the enforcer.
func TestWithMetrics_SetsMetrics(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	m := &telemetry.Metrics{}
	enforcer := NewEnforcer(store, rdb, slog.Default(), WithMetrics(m))
	if enforcer.metrics != m {
		t.Fatal("expected metrics to be set")
	}
}

// TestWithMetrics_OverridesExisting verifies that calling WithMetrics twice
// uses the last value.
func TestWithMetrics_OverridesExisting(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	m1 := &telemetry.Metrics{}
	m2 := &telemetry.Metrics{}
	enforcer := NewEnforcer(store, rdb, slog.Default(), WithMetrics(m1), WithMetrics(m2))
	if enforcer.metrics != m2 {
		t.Fatal("expected last WithMetrics to win")
	}
}

// ---------------------------------------------------------------------------.
// NewEnforcer -- remaining constructor paths
// ---------------------------------------------------------------------------.

// TestNewEnforcer_NilStore_Panics verifies that passing a nil store panics.
func TestNewEnforcer_NilStore_Panics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil store")
		}
	}()
	NewEnforcer(nil, nil, nil)
}

// TestNewEnforcer_NilLogger_UsesDefault verifies that passing a nil logger
// falls back to slog.Default() without panicking.
func TestNewEnforcer_NilLogger_UsesDefault(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	enforcer := NewEnforcer(store, rdb, nil)
	if enforcer.logger == nil {
		t.Fatal("expected logger to be non-nil (should fall back to slog.Default())")
	}
}

// TestNewEnforcer_NilRedis_CreatesEnforcer verifies that the enforcer can be
// created with nil Redis (it will fail open on limit checks).
func TestNewEnforcer_NilRedis_CreatesEnforcer(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())

	if enforcer == nil {
		t.Fatal("expected non-nil enforcer")
	}
	if enforcer.rdb != nil {
		t.Fatal("expected nil rdb")
	}
}

// TestNewEnforcer_WithMultipleOptions verifies that multiple functional
// options are applied in order.
func TestNewEnforcer_WithMultipleOptions(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	m := &telemetry.Metrics{}
	enforcer := NewEnforcer(store, rdb, slog.Default(), WithMetrics(m))
	if enforcer.metrics != m {
		t.Fatal("WithMetrics option was not applied")
	}
	if enforcer.store != store {
		t.Fatal("store not set correctly")
	}
}

// TestNewEnforcer_CacheInitialized verifies that the org cache is properly
// initialized by the constructor (not nil).
func TestNewEnforcer_CacheInitialized(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	enforcer := NewEnforcer(store, rdb, slog.Default())
	if enforcer.orgCache == nil {
		t.Fatal("expected orgCache to be initialized")
	}
}

// ---------------------------------------------------------------------------.
// InvalidateOrgCache -- cache invalidation
// ---------------------------------------------------------------------------.

// TestInvalidateOrgCache_CacheHitThenInvalidate verifies that after populating
// the cache via GetOrgPlanLimits, InvalidateOrgCache clears it.
func TestInvalidateOrgCache_CacheHitThenInvalidate(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	callCount := 0
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			callCount++
			return nil, ErrSubscriptionNotFound
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()

	// First call populates the cache.
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-cache")
	firstCount := callCount

	// Second call should come from cache (no additional store call).
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-cache")
	if callCount != firstCount {
		t.Fatalf("expected cached result, but store was called again (count: %d)", callCount)
	}

	// Invalidate and verify the store is called again.
	enforcer.InvalidateOrgCache("org-cache")
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-cache")
	if callCount <= firstCount {
		t.Fatal("expected store to be called again after cache invalidation")
	}
}
