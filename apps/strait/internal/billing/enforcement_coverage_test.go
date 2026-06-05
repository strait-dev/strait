package billing

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// CheckDailyRunLimit -- remaining branches

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

// TestCheckDailyRunLimit_UnlimitedFreeTier verifies that the free tier has
// unlimited daily runs and no boundary rejection occurs.
func TestCheckDailyRunLimit_UnlimitedFreeTier(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()

	// All plans now have unlimited daily runs (MaxRunsPerDay = -1).
	// Verify many runs succeed without hitting any limit.
	for i := range 10_000 {
		if err := enforcer.CheckDailyRunLimit(ctx, "org-boundary"); err != nil {
			t.Fatalf("unexpected error at run %d: daily runs should be unlimited: %v", i+1, err)
		}
	}
}

// TestCheckDailyRunLimit_DBError_FailsClosed verifies that when the billing
// store returns an error for GetOrgSubscription, the daily limit check returns
// a degraded enforcement error instead of bypassing payment and quota gates.
func TestCheckDailyRunLimit_DBError_FailsClosed(t *testing.T) {
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
	if err == nil {
		t.Fatal("expected fail-closed error on DB error, got nil")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("Code = %q, want service_degraded", le.Code)
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

// DecrDailyRunCount -- decrement paths and error handling

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

// TestDecrDailyRunCount_RollbackWithUnlimitedRuns verifies decrement works
// correctly when daily runs are unlimited (no rejection expected).
func TestDecrDailyRunCount_RollbackWithUnlimitedRuns(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()

	// Run some jobs.
	for range 100 {
		if err := enforcer.CheckDailyRunLimit(ctx, "org-rollback2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Decrement should not panic.
	enforcer.DecrDailyRunCount(ctx, "org-rollback2")

	// Runs should still be allowed (unlimited).
	if err := enforcer.CheckDailyRunLimit(ctx, "org-rollback2"); err != nil {
		t.Fatalf("expected pass after decrement, got: %v", err)
	}
}

// WithMetrics -- functional option

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

// NewEnforcer -- remaining constructor paths

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
		return
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

func TestNewEnforcer_RegistersStrongCacheNamespace(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
		mr.Close()
	})
	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "billing-test"})

	enforcer := NewEnforcer(store, rdb, slog.Default(), WithCacheBus(nil, registry))

	if enforcer.orgCache == nil {
		t.Fatal("expected orgCache to be initialized")
	}
	for _, namespace := range registry.RegisteredNamespaces() {
		if namespace == orgLimitsCacheNamespace {
			return
		}
	}
	t.Fatalf("cache namespace %s was not registered; registered namespaces: %v", orgLimitsCacheNamespace, registry.RegisteredNamespaces())
}

// InvalidateOrgCache -- cache invalidation

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

func TestOrgLimitsCache_PreservesSubscriptionCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	limits := GetPlanLimits(domain.PlanPro)
	raw, err := json.Marshal(limits)
	if err != nil {
		t.Fatalf("marshal limits: %v", err)
	}
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-versioned": {
				ID:              "sub-versioned",
				OrgID:           "org-versioned",
				PlanTier:        string(domain.PlanPro),
				Status:          "active",
				EnforcementMode: "enforce",
				Entitlements:    raw,
				CacheVersion:    12,
			},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	got, err := enforcer.GetOrgPlanLimits(context.Background(), "org-versioned")
	if err != nil {
		t.Fatalf("GetOrgPlanLimits() error = %v", err)
	}
	if got.PlanTier != domain.PlanPro {
		t.Fatalf("PlanTier = %q, want %q", got.PlanTier, domain.PlanPro)
	}

	cached, err := rdb.Get(context.Background(), "strait:cache:"+orgLimitsCacheNamespace+":org-versioned").Bytes()
	if err != nil {
		t.Fatalf("read redis entry: %v", err)
	}
	var envelope struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(cached, &envelope); err != nil {
		t.Fatalf("decode redis entry: %v", err)
	}
	if envelope.Version != 12 {
		t.Fatalf("redis version = %d, want 12", envelope.Version)
	}
}
