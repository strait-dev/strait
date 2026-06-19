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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CheckDailyRunLimit -- remaining branches

// TestCheckDailyRunLimit_NilRedis_FailsOpen verifies that a nil Redis client
// causes CheckDailyRunLimit to return nil (fail open) rather than panic.
func TestCheckDailyRunLimit_NilRedis_FailsOpen(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())
	require.NoError(t,
		enforcer.CheckDailyRunLimit(context.Background(),

			"org-1"))
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
	require.NoError(t,
		err)
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
	for range 10_000 {
		require.NoError(t,
			enforcer.CheckDailyRunLimit(ctx, "org-boundary"),
		)
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
	require.Error(t,
		err)

	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t,
		"service_degraded",
		le.Code,
	)
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
	for range limits.MaxRunsPerDay + 10 {
		require.NoError(t,
			enforcer.CheckDailyRunLimit(ctx, "org-disabled"),
		)
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

	for range limits.MaxRunsPerDay + 5 {
		require.NoError(t,
			enforcer.CheckDailyRunLimit(ctx, "org-warn"))
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
	require.Error(t,
		err)

	var le *LimitError
	require.ErrorAs(t, err, &le)
	assert.Equal(t, "payment_restricted",

		le.Code,
	)
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
	require.NoError(t,
		enforcer.CheckDailyRunLimit(ctx, "org-floor"))

	// Increment once, then decrement twice. Counter should not go negative.

	enforcer.DecrDailyRunCount(ctx, "org-floor")
	enforcer.DecrDailyRunCount(ctx, "org-floor")
	require.NoError(t,
		enforcer.CheckDailyRunLimit(ctx, "org-floor"))

	// Counter should still allow runs (zero or positive).
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
		require.NoError(t,
			enforcer.CheckDailyRunLimit(ctx, "org-rollback2"),
		)
	}

	// Decrement should not panic.
	enforcer.DecrDailyRunCount(ctx, "org-rollback2")
	require.NoError(t,
		enforcer.CheckDailyRunLimit(ctx, "org-rollback2"),
	)

	// Runs should still be allowed (unlimited).
}

// WithMetrics -- functional option

// TestWithMetrics_NilMetrics verifies that passing nil metrics does not panic.
func TestWithMetrics_NilMetrics(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	enforcer := NewEnforcer(store, rdb, slog.Default(), WithMetrics(nil))
	require.Nil(t, enforcer.metrics)
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
	require.Equal(t,
		m, enforcer.metrics,
	)
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
	require.Equal(t,
		m2, enforcer.metrics,
	)
}

// NewEnforcer -- remaining constructor paths

// TestNewEnforcer_NilStore_Panics verifies that passing a nil store panics.
func TestNewEnforcer_NilStore_Panics(t *testing.T) {
	t.Parallel()
	defer func() {
		require.NotNil(
			t, recover(),
		)
	}()
	NewEnforcer(nil, nil, nil)
}

func TestNewEnforcer_TypedNilStore_Panics(t *testing.T) {
	t.Parallel()
	defer func() {
		require.NotNil(
			t, recover(),
		)
	}()

	var store *mockBillingStore
	NewEnforcer(store, nil, nil)
}

// TestNewEnforcer_NilLogger_UsesDefault verifies that passing a nil logger
// falls back to slog.Default() without panicking.
func TestNewEnforcer_NilLogger_UsesDefault(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	enforcer := NewEnforcer(store, rdb, nil)
	require.NotNil(t,
		enforcer.logger)
}

// TestNewEnforcer_NilRedis_CreatesEnforcer verifies that the enforcer can be
// created with nil Redis (it will fail open on limit checks).
func TestNewEnforcer_NilRedis_CreatesEnforcer(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, nil, slog.Default())
	require.NotNil(t,
		enforcer)
	require.Nil(t, enforcer.rdb)
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
	require.Equal(t,
		m, enforcer.metrics,
	)
	require.Equal(t,
		store, enforcer.store,
	)
}

// TestNewEnforcer_CacheInitialized verifies that the org cache is properly
// initialized by the constructor (not nil).
func TestNewEnforcer_CacheInitialized(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	enforcer := NewEnforcer(store, rdb, slog.Default())
	require.NotNil(t,
		enforcer.orgCache,
	)
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
	require.NotNil(t,
		enforcer.orgCache,
	)

	for _, namespace := range registry.RegisteredNamespaces() {
		if namespace == orgLimitsCacheNamespace {
			return
		}
	}
	require.Failf(t, "test failure",

		"cache namespace %s was not registered; registered namespaces: %v", orgLimitsCacheNamespace, registry.RegisteredNamespaces())
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
	require.Equal(t,
		firstCount, callCount,
	)

	// Invalidate and verify the store is called again.
	enforcer.InvalidateOrgCache("org-cache")
	_, _ = enforcer.GetOrgPlanLimits(ctx, "org-cache")
	require.Greater(t,
		callCount, firstCount,
	)
}

func TestOrgLimitsCache_PreservesSubscriptionCacheVersionInRedis(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	limits := GetPlanLimits(domain.PlanPro)
	raw, err := json.Marshal(limits)
	require.NoError(t,
		err)

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
	require.NoError(t,
		err)
	require.Equal(t,
		domain.PlanPro, got.
			PlanTier,
	)

	cached, err := rdb.Get(context.Background(), "strait:cache:"+orgLimitsCacheNamespace+":org-versioned").Bytes()
	require.NoError(t,
		err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(t,
		json.Unmarshal(
			cached, &envelope,
		))
	require.EqualValues(t, 12, envelope.Version)
}
