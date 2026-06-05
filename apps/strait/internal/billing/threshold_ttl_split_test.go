package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUsageThresholdTTLFor_DailyPeriodReturnsShortTTL locks the cadence
// detection: a "YYYY-MM-DD" period is daily and should select the 36h TTL.
func TestUsageThresholdTTLFor_DailyPeriodReturnsShortTTL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, usageThresholdDailyTTL,

		usageThresholdTTLFor("2026-05-10"))
}

// TestUsageThresholdTTLFor_MonthlyPeriodReturnsLongTTL locks the monthly
// branch — "YYYY-MM" must keep the 62-day TTL that survives a long month
// plus clock skew.
func TestUsageThresholdTTLFor_MonthlyPeriodReturnsLongTTL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, usageThresholdMonthlyTTL,

		usageThresholdTTLFor("2026-05"))
}

// TestUsageThresholdTTLFor_UnknownShapeFallsBackToMonthly proves a future
// cadence (hourly, weekly) defaults to the long TTL — a longer-than-needed
// dedupe is safer than one that expires before the window does.
func TestUsageThresholdTTLFor_UnknownShapeFallsBackToMonthly(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"", "2026", "2026-W19", "2026-05-10T00", "weird"} {
		assert.Equal(t, usageThresholdMonthlyTTL,

			usageThresholdTTLFor(p))
	}
}

// TestUsageThresholdTTLFor_DailyTTLNoLongerThan48h is the cost guard. The
// whole point of the split is that a daily key sitting in Redis for 62 days
// is wasted memory. If a future change pushes the daily TTL toward the
// monthly TTL the savings vanish — fail the build before that lands.
func TestUsageThresholdTTLFor_DailyTTLNoLongerThan48h(t *testing.T) {
	t.Parallel()
	assert.LessOrEqual(t, usageThresholdDailyTTL,

		48*
			time.Hour)
}

// TestUsageThresholdTTLFor_MonthlyTTLAtLeast31DaysPlusSkew is the correctness
// guard for the monthly branch. A 31-day month plus a few hours of clock
// skew is the floor; any TTL below that risks re-emitting inside the same
// billing window.
func TestUsageThresholdTTLFor_MonthlyTTLAtLeast31DaysPlusSkew(t *testing.T) {
	t.Parallel()
	assert.GreaterOrEqual(t, usageThresholdMonthlyTTL,

		32*24*time.Hour)
}

// TestMaybeEmitUsageThreshold_DailyKeyHasShortTTL proves the wiring: the
// emit path must hand the daily TTL to Redis when the period is daily-shaped.
// Without this the constants exist but the call site could quietly keep
// using the monthly value.
func TestMaybeEmitUsageThreshold_DailyKeyHasShortTTL(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-daily", "starter", "daily_runs", "2026-05-10", 80, 100)

	key := usageThresholdKey("org-daily", "daily_runs", 80, "2026-05-10")
	ttl := mr.TTL(key)
	require.NotEqual(
		t, 0, ttl)
	assert.LessOrEqual(t, ttl, usageThresholdDailyTTL+
		time.Minute)
	assert.GreaterOrEqual(t, ttl,
		usageThresholdDailyTTL-
			time.Minute)

	// miniredis returns the *remaining* TTL on read; allow some slack but
	// reject any value within an hour of the monthly TTL.
}

// TestMaybeEmitUsageThreshold_MonthlyKeyHasLongTTL is the symmetric guard
// for the monthly branch.
func TestMaybeEmitUsageThreshold_MonthlyKeyHasLongTTL(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-monthly", "pro", "monthly_runs", "2026-05", 80, 100)

	key := usageThresholdKey("org-monthly", "monthly_runs", 80, "2026-05")
	ttl := mr.TTL(key)
	require.NotEqual(
		t, 0, ttl)
	assert.LessOrEqual(t, ttl, usageThresholdMonthlyTTL+
		time.Minute)
	assert.GreaterOrEqual(t, ttl,
		usageThresholdMonthlyTTL-
			time.Minute)
}
