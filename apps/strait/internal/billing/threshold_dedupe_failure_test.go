package billing

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"strait/internal/telemetry"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newDedupeFailureTestEnforcer wires a real Enforcer with a manual-reader
// meter so tests can assert UsageThresholdDedupeFailed actually increments,
// plus a slog handler that captures records so we can assert the level was
// raised from Warn to Error.
func newDedupeFailureTestEnforcer(t *testing.T, rdb redis.Cmdable) (*Enforcer, *metric.ManualReader, *bytes.Buffer) {
	t.Helper()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("billing-dedupe-test")

	dedupe, err := meter.Int64Counter("strait.billing.usage_threshold_dedupe_failed_total")
	if err != nil {
		t.Fatalf("create dedupe counter: %v", err)
	}
	emitted, err := meter.Int64Counter("strait.billing.usage_threshold_emitted_total")
	if err != nil {
		t.Fatalf("create emitted counter: %v", err)
	}
	m := &telemetry.Metrics{
		UsageThresholdDedupeFailed: dedupe,
		UsageThresholdEmitted:      emitted,
	}

	logBuf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	enforcer := NewEnforcer(&mockBillingStore{}, rdb, logger, WithMetrics(m))
	return enforcer, reader, logBuf
}

func dedupeCounterValue(t *testing.T, reader *metric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "strait.billing.usage_threshold_dedupe_failed_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("dedupe metric is not int64 Sum: %T", m.Data)
			}
			for _, dp := range sum.DataPoints {
				key := dedupeAttr(dp.Attributes, "metric") + "|" +
					dedupeAttr(dp.Attributes, "threshold_pct") + "|" +
					dedupeAttr(dp.Attributes, "plan_tier")
				out[key] += dp.Value
			}
		}
	}
	return out
}

func dedupeAttr(set attribute.Set, key string) string {
	v, ok := set.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return v.AsString()
}

// brokenRedis is a tiny redis.Cmdable stand-in whose SetNX always returns an
// error. It satisfies the only call path the threshold emitter exercises.
type brokenRedis struct {
	redis.Cmdable
}

func (b *brokenRedis) SetNX(ctx context.Context, key string, value any, ttl time.Duration) *redis.BoolCmd {
	cmd := redis.NewBoolCmd(ctx, "setnx", key, value)
	cmd.SetErr(errors.New("simulated redis outage"))
	return cmd
}

// TestMaybeEmitUsageThreshold_DedupeFailureIncrementsCounter proves the failure
// path increments the new UsageThresholdDedupeFailed counter under the
// canonical (plan_tier, metric, threshold_pct) attributes. Without this the
// failure path is silent and operators have no way to alert on it.
func TestMaybeEmitUsageThreshold_DedupeFailureIncrementsCounter(t *testing.T) {
	t.Parallel()

	enforcer, reader, _ := newDedupeFailureTestEnforcer(t, &brokenRedis{})
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-broken", "starter", "monthly_runs", "2026-05", 80, 100)

	got := dedupeCounterValue(t, reader)
	if got["monthly_runs|80|starter"] != 1 {
		t.Errorf("expected 1 dedupe failure increment under (monthly_runs, 80, starter), got %v", got)
	}
}

// TestMaybeEmitUsageThreshold_DedupeFailureLogsAtErrorLevel proves the
// Warn→Error level bump landed. Operators wire on-call paging to errors,
// not warnings, and the previous Warn level made dedupe outages invisible
// in dashboards filtered by severity.
func TestMaybeEmitUsageThreshold_DedupeFailureLogsAtErrorLevel(t *testing.T) {
	t.Parallel()

	enforcer, _, logBuf := newDedupeFailureTestEnforcer(t, &brokenRedis{})
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-broken", "starter", "monthly_runs", "2026-05", 80, 100)

	out := logBuf.String()
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("expected Error-level log, got: %s", out)
	}
	if !strings.Contains(out, "usage threshold dedupe failed") {
		t.Errorf("expected dedupe failure message, got: %s", out)
	}
}

// TestMaybeEmitUsageThreshold_DedupeFailureNoCounterWithoutMetrics guards the
// nil-metrics path used by tests and community edition: the Error log must
// still fire, and the function must not panic when metrics are nil.
func TestMaybeEmitUsageThreshold_DedupeFailureNoCounterWithoutMetrics(t *testing.T) {
	t.Parallel()

	logBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	enforcer := NewEnforcer(&mockBillingStore{}, &brokenRedis{}, logger)

	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-broken", "free", "daily_runs", "2026-05-10", 100, 100)

	if !strings.Contains(logBuf.String(), `"level":"ERROR"`) {
		t.Errorf("expected Error-level log even without metrics, got: %s", logBuf.String())
	}
}

// TestMaybeEmitUsageThreshold_HealthyRedisDoesNotIncrementDedupeFailure scopes
// the new counter: a successful SETNX must NOT bump dedupe-failed. Without
// this assertion the counter could double as the emitted-total counter and
// silently corrupt billing dashboards.
func TestMaybeEmitUsageThreshold_HealthyRedisDoesNotIncrementDedupeFailure(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer, reader, _ := newDedupeFailureTestEnforcer(t, rdb)
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-healthy", "pro", "monthly_runs", "2026-05", 80, 100)

	got := dedupeCounterValue(t, reader)
	for k, v := range got {
		if v != 0 {
			t.Errorf("healthy redis must not increment dedupe-failed, got %s=%d", k, v)
		}
	}
}
