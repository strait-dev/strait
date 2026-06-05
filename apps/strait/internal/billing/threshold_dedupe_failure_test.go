package billing

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

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

// newDedupeFailureTestEnforcer wires a real Enforcer with a slog handler that
// captures records so tests can assert the log level was raised from Warn to
// Error. The metric increment is exercised at runtime via the package-level
// recordBillingUsageThresholdDedupeFailed helper; tests rely on the build-tag
// no-panic coverage in metrics_build_tags_test.go for that side effect.
func newDedupeFailureTestEnforcer(t *testing.T, rdb redis.Cmdable) (*Enforcer, *bytes.Buffer) {
	t.Helper()
	logBuf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	enforcer := NewEnforcer(&mockBillingStore{}, rdb, logger)
	return enforcer, logBuf
}

// TestMaybeEmitUsageThreshold_DedupeFailureLogsAtErrorLevel proves the
// Warn→Error level bump landed. Operators wire on-call paging to errors,
// not warnings, and the previous Warn level made dedupe outages invisible
// in dashboards filtered by severity.
func TestMaybeEmitUsageThreshold_DedupeFailureLogsAtErrorLevel(t *testing.T) {
	t.Parallel()

	enforcer, logBuf := newDedupeFailureTestEnforcer(t, &brokenRedis{})
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-broken", "starter", "monthly_runs", "2026-05", 80, 100)

	out := logBuf.String()
	assert.True(t,
		strings.Contains(out,
			`"level":"ERROR"`,
		))
	assert.True(t,
		strings.Contains(out,
			"usage threshold dedupe failed",
		))

}

// TestMaybeEmitUsageThreshold_DedupeFailureNoCounterWithoutMetrics guards the
// code path that runs without injected metrics: the Error log must still
// fire and the function must not panic.
func TestMaybeEmitUsageThreshold_DedupeFailureNoCounterWithoutMetrics(t *testing.T) {
	t.Parallel()

	logBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	enforcer := NewEnforcer(&mockBillingStore{}, &brokenRedis{}, logger)

	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-broken", "free", "daily_runs", "2026-05-10", 100, 100)
	assert.True(t,
		strings.Contains(logBuf.
			String(),
			`"level":"ERROR"`,
		))

}

// TestMaybeEmitUsageThreshold_HealthyRedisDoesNotLogError proves the success
// path stays quiet on the Error stream — only failures should page on-call.
func TestMaybeEmitUsageThreshold_HealthyRedisDoesNotLogError(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	enforcer, logBuf := newDedupeFailureTestEnforcer(t, rdb)
	enforcer.maybeEmitUsageThreshold(context.Background(),
		"org-healthy", "pro", "monthly_runs", "2026-05", 80, 100)
	assert.False(t,
		strings.Contains(logBuf.
			String(),
			`"level":"ERROR"`,
		))

}
