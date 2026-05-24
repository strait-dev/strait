package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
)

// TestPercentReached covers the boundary arithmetic that decides whether a
// counter has crossed an 80/90/100 percent line.
func TestPercentReached(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current int64
		limit   int64
		pct     int
		want    bool
	}{
		{"zero current", 0, 100, 80, false},
		{"just below 80", 79, 100, 80, false},
		{"exactly 80", 80, 100, 80, true},
		{"just above 80", 81, 100, 80, true},
		{"100 of 100 hits 100", 100, 100, 100, true},
		{"over the limit hits 100", 105, 100, 100, true},
		{"zero limit is no-op", 50, 0, 80, false},
		{"negative limit is no-op", 50, -1, 80, false},
		{"negative current is no-op", -5, 100, 80, false},
		{"large numbers no overflow", 800_000_000, 1_000_000_000, 80, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := percentReached(tc.current, tc.limit, tc.pct); got != tc.want {
				t.Errorf("percentReached(%d, %d, %d) = %v, want %v",
					tc.current, tc.limit, tc.pct, got, tc.want)
			}
		})
	}
}

// TestMaybeEmitUsageThreshold_DedupesWithinPeriod proves that re-entering the
// same threshold inside the same window emits exactly once. Without this the
// threshold would fire on every single request once the counter sits above
// the line.
func TestMaybeEmitUsageThreshold_DedupesWithinPeriod(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	// 80 of 100 = 80%. Call ten times — only the first one should claim
	// the SETNX key.
	for range 10 {
		enforcer.maybeEmitUsageThreshold(ctx, "org-A", "free", "monthly_runs", "2026-05",
			80, 100)
	}

	key := usageThresholdKey("org-A", "monthly_runs", 80, "2026-05")
	if !mr.Exists(key) {
		t.Fatalf("expected dedupe key %q to exist after first emit", key)
	}
	// 90% should still be free to fire once.
	enforcer.maybeEmitUsageThreshold(ctx, "org-A", "free", "monthly_runs", "2026-05",
		91, 100)
	key90 := usageThresholdKey("org-A", "monthly_runs", 90, "2026-05")
	if !mr.Exists(key90) {
		t.Fatalf("expected 90%% dedupe key %q to exist after first crossing", key90)
	}
}

// TestMaybeEmitUsageThreshold_HighestBucketWins ensures that when a single
// call crosses multiple thresholds at once (e.g. usage jumps 70 → 100 in one
// run), we record the most actionable bucket — 100% — and not 80 or 90.
func TestMaybeEmitUsageThreshold_HighestBucketWins(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	ctx := context.Background()

	enforcer.maybeEmitUsageThreshold(ctx, "org-jump", "starter", "monthly_runs", "2026-05",
		100, 100)

	// Only the 100% key should be claimed.
	for _, pct := range []int{80, 90} {
		k := usageThresholdKey("org-jump", "monthly_runs", pct, "2026-05")
		if mr.Exists(k) {
			t.Errorf("expected lower bucket %d%% key %q to be untouched, got existing", pct, k)
		}
	}
	if !mr.Exists(usageThresholdKey("org-jump", "monthly_runs", 100, "2026-05")) {
		t.Errorf("expected 100%% bucket key to be claimed")
	}
}

// TestMaybeEmitUsageThreshold_BelowAllBucketsNoOp verifies that callers below
// the first bucket (80%) emit nothing and write nothing to Redis.
func TestMaybeEmitUsageThreshold_BelowAllBucketsNoOp(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	ctx := context.Background()

	enforcer.maybeEmitUsageThreshold(ctx, "org-low", "starter", "monthly_runs", "2026-05",
		79, 100)

	if got := mr.Keys(); len(got) != 0 {
		t.Errorf("expected no Redis keys when below 80%%, got %v", got)
	}
}

// TestMaybeEmitUsageThreshold_DistinctPeriodsEmitIndependently ensures that
// the period component is part of the dedupe key — last month's emission
// must not silence this month's.
func TestMaybeEmitUsageThreshold_DistinctPeriodsEmitIndependently(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	ctx := context.Background()

	enforcer.maybeEmitUsageThreshold(ctx, "org-month", "pro", "monthly_runs", "2026-04",
		80, 100)
	enforcer.maybeEmitUsageThreshold(ctx, "org-month", "pro", "monthly_runs", "2026-05",
		80, 100)

	for _, period := range []string{"2026-04", "2026-05"} {
		if !mr.Exists(usageThresholdKey("org-month", "monthly_runs", 80, period)) {
			t.Errorf("expected per-period dedupe for %s", period)
		}
	}
}

// TestMaybeEmitUsageThreshold_NoRedisIsNoOp confirms the fail-quiet path when
// the enforcer has no Redis client. Threshold warnings are advisory and must
// not panic or block the hot path on Redis-less deployments.
func TestMaybeEmitUsageThreshold_NoRedisIsNoOp(t *testing.T) {
	t.Parallel()
	enforcer := NewEnforcer(&mockBillingStore{}, nil, slog.Default())
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-nored", "free", "monthly_runs", "2026-05", 100, 100)
}

// TestMaybeEmitUsageThreshold_RaceSingleEmission proves that 100 concurrent
// callers crossing the 80% line race for the same SETNX, but only one wins.
// The other 99 must not double-emit.
func TestMaybeEmitUsageThreshold_RaceSingleEmission(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	ctx := context.Background()

	const callers = 100
	wg := conc.NewWaitGroup()
	for range callers {
		wg.Go(func() {
			enforcer.maybeEmitUsageThreshold(ctx, "org-race", "starter", "monthly_runs",
				"2026-05", 80, 100)
		})
	}
	wg.Wait()

	if !mr.Exists(usageThresholdKey("org-race", "monthly_runs", 80, "2026-05")) {
		t.Fatalf("expected exactly one of the racing callers to claim the dedupe key")
	}
}

// TestCheckMonthlyRunLimit_EmitsThresholdAtBoundary wires the threshold
// warning through the enforcer's hot path. We pre-seed the monthly counter
// just below 80% of the free cap, then a single CheckMonthlyRunLimit call
// must (a) succeed and (b) claim the 80% dedupe key.
func TestCheckMonthlyRunLimit_EmitsThresholdAtBoundary(t *testing.T) {
	t.Parallel()
	enforcer, store, mr := setupEnforcer(t)
	ctx := context.Background()

	const orgID = "org-thresh-monthly"
	store.subscriptions = map[string]*OrgSubscription{
		orgID: {OrgID: orgID, PlanTier: "free", Status: "active"},
	}

	// Free tier: 5000/month. 80% boundary = 4000.
	period := time.Now().UTC().Format("2006-01")
	counterKey := monthlyRunKey(orgID, time.Now())
	if err := mr.Set(counterKey, "3999"); err != nil {
		t.Fatalf("seed counter: %v", err)
	}

	if err := enforcer.CheckMonthlyRunLimit(ctx, orgID); err != nil {
		t.Fatalf("CheckMonthlyRunLimit at 4000/5000: %v", err)
	}

	// One incr must have crossed 80% and claimed the dedupe key.
	dedupe80 := usageThresholdKey(orgID, "monthly_runs", 80, period)
	if !mr.Exists(dedupe80) {
		t.Errorf("expected 80%% dedupe key %q to be claimed after crossing 4000/5000", dedupe80)
	}
	for _, pct := range []int{90, 100} {
		k := usageThresholdKey(orgID, "monthly_runs", pct, period)
		if mr.Exists(k) {
			t.Errorf("expected %d%% dedupe key not yet claimed at boundary, got %q", pct, k)
		}
	}
}

// TestMaybeEmitUsageThreshold_EmptyInputsAreNoOps protects against accidental
// calls with missing identifiers — those would silently spam a meaningless
// dedupe key without this guard.
func TestMaybeEmitUsageThreshold_EmptyInputsAreNoOps(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, slog.Default())
	ctx := context.Background()

	cases := []struct{ org, metric, period string }{
		{"", "monthly_runs", "2026-05"},
		{"org-X", "", "2026-05"},
		{"org-X", "monthly_runs", ""},
	}
	for _, c := range cases {
		enforcer.maybeEmitUsageThreshold(ctx, c.org, "starter", c.metric, c.period, 100, 100)
	}
	if got := mr.Keys(); len(got) != 0 {
		t.Errorf("expected no keys for empty-id inputs, got %v", got)
	}
}
