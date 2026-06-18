package billing

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"
)

// thresholdPercents enumerates the boundaries we notify on as the org's
// metered usage approaches its plan cap. Order matters: maybeEmitUsageThreshold
// only emits the highest crossed bucket so an org racing past 80 → 100 in a
// single call records the most actionable signal, not three of them.
//
// Declared as an array (not a slice) so callers cannot mutate it at runtime.
// The constructor below also rejects any future edit that breaks the
// strictly-ascending order the highest-crossed scan depends on.
var thresholdPercents = mustAscendingThresholdPercents([...]int{80, 90, 100})

func mustAscendingThresholdPercents(percents [3]int) [3]int {
	for i := 1; i < len(percents); i++ {
		if percents[i] <= percents[i-1] {
			panic(fmt.Sprintf(
				"billing: threshold percents must be strictly ascending; got %v",
				percents,
			))
		}
	}
	if percents[0] <= 0 {
		panic(fmt.Sprintf(
			"billing: threshold percents must start above 0; got %v",
			percents,
		))
	}
	return percents
}

// usageThresholdKey is the Redis SETNX key that dedupes a (org, metric,
// percent, period) emission. The period component gates the window — daily
// metrics use the calendar day in UTC, monthly metrics use the calendar
// month — so a fresh window starts from zero with no manual reset.
func usageThresholdKey(orgID, metricName string, pct int, period string) string {
	return fmt.Sprintf("strait:usage_threshold:%s:%s:%d:%s", orgID, metricName, pct, period)
}

// usageThresholdMonthlyTTL keeps a monthly-window dedupe entry alive long
// enough to outlast any single billing month. 62 days covers monthly windows
// even when a billing period straddles a 31-day month with a few hours of
// clock skew on either side.
func usageThresholdMonthlyTTL() time.Duration {
	return 62 * 24 * time.Hour
}

// usageThresholdDailyTTL keeps a daily-window dedupe entry alive 36 hours.
// The period component (e.g. "2026-05-10") rotates at UTC midnight so the
// next day starts from a different key regardless of TTL, but a 36h window
// covers worst-case clock skew between the writer and a follow-up reader on
// the previous day's key. Sticking the previous monthly TTL on a daily key
// would leave 90% of the keyspace as garbage that Redis must eventually
// evict, costing memory for nothing.
func usageThresholdDailyTTL() time.Duration {
	return 36 * time.Hour
}

// dailyPeriodLen is len("YYYY-MM-DD"). Monthly periods use len("YYYY-MM")
// (7 chars). The period string is the only signal we have at the dedupe
// site; treat any length other than the daily one as monthly so an unknown
// future cadence defaults to the longer, safer TTL.
func dailyPeriodLen() int {
	return len("2006-01-02")
}

// usageThresholdTTLFor selects the dedupe TTL based on the period string
// shape. Centralising the choice here means a future cadence (hourly,
// weekly) lands in one place instead of being scattered through the emit
// path.
func usageThresholdTTLFor(period string) time.Duration {
	if len(period) == dailyPeriodLen() {
		return usageThresholdDailyTTL()
	}
	return usageThresholdMonthlyTTL()
}

// maybeEmitUsageThreshold records a one-shot 80/90/100% threshold notification
// for a metered counter. It is safe to call from the hot path of every
// CheckXxxLimit: the SETNX dedupe holds even under concurrent callers, and the
// only side effects on the duplicate path are a single Redis round-trip.
//
// metricName is the short tag (e.g. "monthly_runs") that flows into ClickHouse,
// the audit registry, and the Prometheus label. period is the calendar key
// that scopes dedupe — pass "2026-05" for monthly counters and "2026-05-10"
// for daily counters.
//
// Crossings are evaluated against the *new* value after the increment, so a
// counter that lands on 80 emits once, then stays silent until it lands on 90.
// Limits of -1 (unlimited) are no-ops; zero or negative limits are no-ops to
// avoid divide-by-zero.
func (e *Enforcer) maybeEmitUsageThreshold(
	ctx context.Context,
	orgID, planTier, metricName, period string,
	current, limit int64,
) {
	if e == nil || orgID == "" || metricName == "" || period == "" {
		return
	}
	if limit <= 0 {
		return
	}

	// Cross-list scan from highest to lowest. We emit the single highest
	// percent the new count crosses; lower buckets are then claimed in
	// Redis so a later call that lands on the same bucket is a no-op.
	highest := -1
	for _, pct := range thresholdPercents {
		if percentReached(current, limit, pct) {
			highest = pct
		}
	}
	if highest < 0 {
		return
	}

	if e.rdb == nil {
		// No dedupe store — skip silently rather than spamming the
		// downstream channels. Threshold warnings are advisory; missing
		// them on a Redis-less deployment is acceptable.
		return
	}

	key := usageThresholdKey(orgID, metricName, highest, period)
	set, err := e.rdb.SetNX(ctx, key, "1", usageThresholdTTLFor(period)).Result()
	if err != nil {
		// Failing the dedupe SETNX means we cannot tell whether this
		// crossing has already been notified. The previous behavior was
		// to silently swallow the error at Warn level, but the result is
		// either a missed customer-facing notification or (after a retry
		// path is added) a duplicate one. Both are noisy in production
		// and worth paging on, so emit at Error and bump a dedicated
		// counter that operators can alert on without grep-on-warn.
		e.logger.Error("usage threshold dedupe failed",
			"org_id", orgID,
			"metric", metricName,
			"threshold_pct", highest,
			"error", err,
		)
		recordBillingUsageThresholdDedupeFailed(ctx, planTier, metricName, strconv.Itoa(highest))
		return
	}
	if !set {
		return // already emitted for this (org, metric, threshold, period)
	}

	recordBillingUsageThresholdEmitted(ctx, planTier, metricName, strconv.Itoa(highest))

	e.emitBillingEvent(orgID, "usage_threshold_"+strconv.Itoa(highest), planTier)

	e.logger.Info("usage threshold reached",
		"org_id", orgID,
		"plan_tier", planTier,
		"metric", metricName,
		"threshold_pct", highest,
		"current", current,
		"limit", limit,
	)
}

// percentReached reports whether current is at or above pct% of limit. We
// avoid floating point by cross-multiplying: current/limit >= pct/100 iff
// current*100 >= limit*pct. Both sides fit comfortably in int64 for any
// realistic usage counter, but we still saturate on overflow so a buggy
// caller passing math.MaxInt64 cannot wrap into negative space and return
// the wrong answer.
func percentReached(current, limit int64, pct int) bool {
	if limit <= 0 || current < 0 || pct <= 0 {
		return false
	}
	// current * 100 overflows int64 when current > MaxInt64/100. Anything
	// past that threshold is, by definition, far beyond any plan cap, so
	// saturate to true (definitely reached).
	if current > math.MaxInt64/100 {
		return true
	}
	// limit * pct overflows int64 when limit > MaxInt64/pct. The crossing
	// is mathematically valid but unreachable in int64 — saturate to
	// false (impossible to reach an effectively-infinite threshold).
	if limit > math.MaxInt64/int64(pct) {
		return false
	}
	return current*100 >= limit*int64(pct)
}
