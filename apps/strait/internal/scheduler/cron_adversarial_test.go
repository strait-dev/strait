package scheduler

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/robfig/cron/v3"
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
	if err == nil {
		t.Fatal("expected error for 10KB cron expression, got nil")
	}
}

// TestCron_ExtremeFrequency verifies that an every-minute expression parses and
// produces a next time within 60 seconds.
func TestCron_ExtremeFrequency(t *testing.T) {
	t.Parallel()

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse("* * * * *")
	if err != nil {
		t.Fatalf("every-minute cron should parse: %v", err)
	}

	now := time.Now()
	next := sched.Next(now)
	delta := next.Sub(now)
	if delta > 60*time.Second || delta < 0 {
		t.Fatalf("expected next fire within 60s, got %v", delta)
	}
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
		if err == nil {
			t.Errorf("expected error for invalid cron %q, got nil", expr)
		}
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
		if err == nil {
			t.Errorf("expected error for unicode digit cron %q, got nil", expr)
		}
	}
}

// TestSLOEvaluator_BoundaryThreshold tests CalculateErrorBudget exactly at SLO target.
func TestSLOEvaluator_BoundaryThreshold(t *testing.T) {
	t.Parallel()

	// Exactly at target for success rate: budget should be 0.
	budget := CalculateErrorBudget(0.99, 0.99, domain.SLOMetricSuccessRate)
	if budget != 0.0 {
		t.Errorf("budget at exact target should be 0.0, got %v", budget)
	}

	// Exactly at target for latency: budget should be 0.
	budget = CalculateErrorBudget(1.0, 1.0, domain.SLOMetricP95LatencySecs)
	if budget != 0.0 {
		t.Errorf("latency budget at exact target should be 0.0, got %v", budget)
	}

	// Epsilon better than target should yield a tiny positive budget.
	budget = CalculateErrorBudget(0.99+1e-10, 0.99, domain.SLOMetricSuccessRate)
	if budget <= 0.0 {
		t.Errorf("budget slightly above target should be positive, got %v", budget)
	}
}

// TestSLOEvaluator_ZeroSamples tests CalculateErrorBudget with zero-value inputs.
func TestSLOEvaluator_ZeroSamples(t *testing.T) {
	t.Parallel()

	// Zero current, zero target for success rate.
	budget := CalculateErrorBudget(0.0, 0.0, domain.SLOMetricSuccessRate)
	if budget < 0 || budget > 1 {
		t.Fatalf("budget out of [0,1] for zero/zero success: %v", budget)
	}

	// Zero current, zero target for latency.
	budget = CalculateErrorBudget(0.0, 0.0, domain.SLOMetricP95LatencySecs)
	if budget < 0 || budget > 1 {
		t.Fatalf("budget out of [0,1] for zero/zero latency: %v", budget)
	}

	// Unknown metric always returns 1.0.
	budget = CalculateErrorBudget(0.0, 0.0, "unknown_metric")
	if budget != 1.0 {
		t.Errorf("unknown metric budget should be 1.0, got %v", budget)
	}
}

// TestSLOEvaluator_NegativeLatency tests that negative latency values produce a clamped budget.
func TestSLOEvaluator_NegativeLatency(t *testing.T) {
	t.Parallel()

	budget := CalculateErrorBudget(-5.0, 1.0, domain.SLOMetricP95LatencySecs)
	if budget < 0 || budget > 1 {
		t.Fatalf("budget out of [0,1] for negative latency: %v", budget)
	}

	budget = CalculateErrorBudget(-1.0, 0.99, domain.SLOMetricSuccessRate)
	if budget < 0 || budget > 1 {
		t.Fatalf("budget out of [0,1] for negative success rate: %v", budget)
	}

	// NaN and Inf should not appear.
	budget = CalculateErrorBudget(math.Inf(1), 1.0, domain.SLOMetricP95LatencySecs)
	if math.IsNaN(budget) {
		t.Fatal("budget should not be NaN for Inf latency")
	}
}

// TestReaper_ZeroRetention verifies that NewReaper clamps zero retention to defaults.
func TestReaper_ZeroRetention(t *testing.T) {
	t.Parallel()

	rs := &mockReaperStore{}
	r := NewReaper(rs, time.Minute, time.Hour, 0, 0, true, nil)

	// Zero values should be clamped to 30d and 90d respectively.
	if r.shortRetention != 30*24*time.Hour {
		t.Errorf("expected short retention clamped to 30d, got %v", r.shortRetention)
	}
	if r.longRetention != 90*24*time.Hour {
		t.Errorf("expected long retention clamped to 90d, got %v", r.longRetention)
	}
}

// TestReaper_NegativeRetention verifies that negative retention is clamped to defaults.
func TestReaper_NegativeRetention(t *testing.T) {
	t.Parallel()

	rs := &mockReaperStore{}
	r := NewReaper(rs, time.Minute, time.Hour, -time.Hour, -time.Hour, true, nil)

	if r.shortRetention != 30*24*time.Hour {
		t.Errorf("expected negative short retention clamped to 30d, got %v", r.shortRetention)
	}
	if r.longRetention != 90*24*time.Hour {
		t.Errorf("expected negative long retention clamped to 90d, got %v", r.longRetention)
	}
}

// TestReaper_MaxRetention verifies that math.MaxInt64 duration does not panic.
func TestReaper_MaxRetention(t *testing.T) {
	t.Parallel()

	rs := &mockReaperStore{}
	r := NewReaper(rs, time.Minute, time.Hour, math.MaxInt64, math.MaxInt64, true, nil)

	if r.shortRetention != math.MaxInt64 {
		t.Errorf("expected MaxInt64 short retention, got %v", r.shortRetention)
	}
	if r.longRetention != math.MaxInt64 {
		t.Errorf("expected MaxInt64 long retention, got %v", r.longRetention)
	}
}

// TestDebouncePoller_ZeroInterval verifies that a zero interval is clamped to 1 second.
func TestDebouncePoller_ZeroInterval(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{}
	q := &mockQueue{}
	p := NewDebouncePoller(ds, q, 0)

	if p.interval != time.Second {
		t.Errorf("expected zero interval clamped to 1s, got %v", p.interval)
	}
}

// TestDebouncePoller_SubMillisecond verifies that sub-millisecond intervals are clamped.
func TestDebouncePoller_SubMillisecond(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{}
	q := &mockQueue{}

	// Negative interval should be clamped.
	p := NewDebouncePoller(ds, q, -time.Microsecond)
	if p.interval != time.Second {
		t.Errorf("expected negative interval clamped to 1s, got %v", p.interval)
	}

	// Valid sub-millisecond positive interval should be accepted as-is.
	p = NewDebouncePoller(ds, q, 500*time.Microsecond)
	if p.interval != 500*time.Microsecond {
		t.Errorf("expected 500us interval preserved, got %v", p.interval)
	}

	// Verify Run does not panic with a sub-millisecond interval by running briefly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	p.Run(ctx)
}
