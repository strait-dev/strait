package billing

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// thresholdPercents enumerates the boundaries we notify on as the org's
// metered usage approaches its plan cap. Order matters: maybeEmitUsageThreshold
// only emits the highest crossed bucket so an org racing past 80 → 100 in a
// single call records the most actionable signal, not three of them.
var thresholdPercents = []int{80, 90, 100}

// usageThresholdKey is the Redis SETNX key that dedupes a (org, metric,
// percent, period) emission. The period component gates the window — daily
// metrics use the calendar day in UTC, monthly metrics use the calendar
// month — so a fresh window starts from zero with no manual reset.
func usageThresholdKey(orgID, metricName string, pct int, period string) string {
	return fmt.Sprintf("strait:usage_threshold:%s:%s:%d:%s", orgID, metricName, pct, period)
}

// usageThresholdTTL keeps the dedupe entry alive long enough to outlast any
// single billing window. 62 days covers monthly windows even when a billing
// period straddles a 31-day month with a few hours of clock skew on either
// side. Daily windows reset naturally because the period component changes
// and a fresh key is written.
const usageThresholdTTL = 62 * 24 * time.Hour

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
	set, err := e.rdb.SetNX(ctx, key, "1", usageThresholdTTL).Result()
	if err != nil {
		e.logger.Warn("usage threshold dedupe failed",
			"org_id", orgID,
			"metric", metricName,
			"threshold_pct", highest,
			"error", err,
		)
		return
	}
	if !set {
		return // already emitted for this (org, metric, threshold, period)
	}

	if e.metrics != nil && e.metrics.UsageThresholdEmitted != nil {
		e.metrics.UsageThresholdEmitted.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("plan_tier", planTier),
				attribute.String("metric", metricName),
				attribute.String("threshold_pct", strconv.Itoa(highest)),
			),
		)
	}

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
// realistic usage counter.
func percentReached(current, limit int64, pct int) bool {
	if limit <= 0 || current < 0 {
		return false
	}
	return current*100 >= limit*int64(pct)
}
