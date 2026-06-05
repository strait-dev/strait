package scheduler

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzCronExpressionReDoS fuzzes cron expression parsing for ReDoS or panics.
func FuzzCronExpressionReDoS(f *testing.F) {
	f.Add("* * * * *")
	f.Add("*/5 * * * *")
	f.Add("0 0 1 1 *")
	f.Add("59 23 31 12 7")
	f.Add("0-59/2 0-23 1-31 1-12 0-7")
	f.Add("@every 1s")
	f.Add("@hourly")
	f.Add(strings.Repeat("*", 1000))
	f.Add("0/0/0/0/0 * * * *")

	f.Fuzz(func(t *testing.T, expr string) {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		// Must not panic regardless of input.
		_, _ = parser.Parse(expr)
	})
}

// TestCron_ExtremelyLong verifies that a 10KB cron string is rejected without panic.
func TestCron_ExtremelyLong(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("* ", 5000)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(long)
	require.Error(t, err)

}

// TestCron_ExtremeFrequency verifies that an every-minute expression parses and
// produces a next time within 60 seconds.
func TestCron_ExtremeFrequency(t *testing.T) {
	t.Parallel()

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse("* * * * *")
	require.NoError(t,
		err)

	now := time.Now()
	next := sched.Next(now)
	delta := next.Sub(now)
	require.False(t, delta >
		60*
			time.Second ||
		delta <
			0)

}

// TestCron_InvalidFields verifies that non-numeric and special characters are rejected.
func TestCron_InvalidFields(t *testing.T) {
	t.Parallel()

	invalids := []string{
		"abc def ghi jkl mno",
		"!@# $%^ &*( )_+ ~",
		"1.5 2.5 3.5 4.5 5.5",
		"-- -- -- -- --",
		"1e3 * * * *",
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for _, expr := range invalids {
		_, err := parser.Parse(expr)
		assert.Error(t, err)

	}
}

// TestCron_NullBytes verifies that null bytes in a cron expression do not cause panics.
func TestCron_NullBytes(t *testing.T) {
	t.Parallel()

	expressions := []string{
		"*\x00* * * *",
		"\x00\x00\x00\x00\x00",
		"* * *\x00* *",
		string([]byte{0, 0, 32, 42, 32, 42, 32, 42, 32, 42}),
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for _, expr := range expressions {
		// Must not panic.
		_, _ = parser.Parse(expr)
	}
}

// TestCron_UnicodeDigits verifies that fullwidth unicode digits are not silently accepted.
func TestCron_UnicodeDigits(t *testing.T) {
	t.Parallel()

	// Fullwidth digits U+FF10-FF19.
	expressions := []string{
		"\uFF10 \uFF10 \uFF11 \uFF11 *",
		"\uFF15 * * * *",
		"* * * * \uFF10",
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for _, expr := range expressions {
		_, err := parser.Parse(expr)
		assert.Error(t, err)

	}
}

// TestSLOEvaluator_BoundaryThreshold tests CalculateErrorBudget exactly at SLO target.
func TestSLOEvaluator_BoundaryThreshold(t *testing.T) {
	t.Parallel()

	// Exactly at target for success rate: budget should be 0.
	budget := CalculateErrorBudget(0.99, 0.99, domain.SLOMetricSuccessRate)
	assert.EqualValues(t, 0.0,
		budget,
	)

	// Exactly at target for latency: budget should be 0.
	budget = CalculateErrorBudget(1.0, 1.0, domain.SLOMetricP95LatencySecs)
	assert.EqualValues(t, 0.0,
		budget,
	)

	// Epsilon better than target should yield a tiny positive budget.
	budget = CalculateErrorBudget(0.99+1e-10, 0.99, domain.SLOMetricSuccessRate)
	assert.False(t, budget <=
		0.0,
	)

}

// TestSLOEvaluator_ZeroSamples tests CalculateErrorBudget with zero-value inputs.
func TestSLOEvaluator_ZeroSamples(t *testing.T) {
	t.Parallel()

	// Zero current, zero target for success rate.
	budget := CalculateErrorBudget(0.0, 0.0, domain.SLOMetricSuccessRate)
	require.False(t, budget <
		0 ||
		budget >
			1)

	// Zero current, zero target for latency.
	budget = CalculateErrorBudget(0.0, 0.0, domain.SLOMetricP95LatencySecs)
	require.False(t, budget <
		0 ||
		budget >
			1)

	// Unknown metric always returns 1.0.
	budget = CalculateErrorBudget(0.0, 0.0, "unknown_metric")
	assert.EqualValues(t, 1.0,
		budget,
	)

}

// TestSLOEvaluator_NegativeLatency tests that negative latency values produce a clamped budget.
func TestSLOEvaluator_NegativeLatency(t *testing.T) {
	t.Parallel()

	budget := CalculateErrorBudget(-5.0, 1.0, domain.SLOMetricP95LatencySecs)
	require.False(t, budget <
		0 ||
		budget >
			1)

	budget = CalculateErrorBudget(-1.0, 0.99, domain.SLOMetricSuccessRate)
	require.False(t, budget <
		0 ||
		budget >
			1)

	// NaN and Inf should not appear.
	budget = CalculateErrorBudget(math.Inf(1), 1.0, domain.SLOMetricP95LatencySecs)
	require.False(t, math.
		IsNaN(budget))

}

// TestReaper_ZeroRetention verifies that NewReaper clamps zero retention to defaults.
func TestReaper_ZeroRetention(t *testing.T) {
	t.Parallel()

	rs := &mockReaperStore{}
	r := NewReaper(rs, time.Minute, time.Hour, 0, 0, true, nil)
	assert.Equal(t, 30*
		24*time.
		Hour, r.
		shortRetention,
	)
	assert.Equal(t, 90*
		24*time.
		Hour, r.
		longRetention,
	)

	// Zero values should be clamped to 30d and 90d respectively.

}

// TestReaper_NegativeRetention verifies that negative retention is clamped to defaults.
func TestReaper_NegativeRetention(t *testing.T) {
	t.Parallel()

	rs := &mockReaperStore{}
	r := NewReaper(rs, time.Minute, time.Hour, -time.Hour, -time.Hour, true, nil)
	assert.Equal(t, 30*
		24*time.
		Hour, r.
		shortRetention,
	)
	assert.Equal(t, 90*
		24*time.
		Hour, r.
		longRetention,
	)

}

// TestReaper_MaxRetention verifies that math.MaxInt64 duration does not panic.
func TestReaper_MaxRetention(t *testing.T) {
	t.Parallel()

	rs := &mockReaperStore{}
	r := NewReaper(rs, time.Minute, time.Hour, math.MaxInt64, math.MaxInt64, true, nil)
	assert.Equal(t, time.Duration(math.
		MaxInt64,
	),
		r.shortRetention,
	)
	assert.Equal(t, time.Duration(math.
		MaxInt64,
	),
		r.longRetention,
	)

}

// TestDebouncePoller_ZeroInterval verifies that a zero interval is clamped to 1 second.
func TestDebouncePoller_ZeroInterval(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{}
	q := &mockQueue{}
	p := NewDebouncePoller(ds, q, 0)
	assert.Equal(t, time.
		Second,
		p.interval,
	)

}

// TestDebouncePoller_SubMillisecond verifies that sub-millisecond intervals are clamped.
func TestDebouncePoller_SubMillisecond(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{}
	q := &mockQueue{}

	// Negative interval should be clamped.
	p := NewDebouncePoller(ds, q, -time.Microsecond)
	assert.Equal(t, time.
		Second,
		p.interval,
	)

	// Valid sub-millisecond positive interval should be accepted as-is.
	p = NewDebouncePoller(ds, q, 500*time.Microsecond)
	assert.Equal(t, 500*
		time.
			Microsecond,
		p.interval,
	)

	// Verify Run does not panic with a sub-millisecond interval by running briefly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	p.Run(ctx)
}
