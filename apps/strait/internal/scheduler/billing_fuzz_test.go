package scheduler

import (
	"context"
	"math"
	"testing"
	"time"
	"unicode/utf8"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// Section separator.
// Fuzz tests: verify no panics or NaN/Inf results with random inputs.
// Section separator.

func FuzzSLOEvaluator_ErrorBudget(f *testing.F) {
	f.Add(0.99, 0.999, "success_rate")
	f.Add(0.0, 0.0, "success_rate")
	f.Add(1.0, 1.0, "success_rate")
	f.Add(0.5, 0.99, "success_rate")
	f.Add(-1.0, 0.99, "success_rate")
	f.Add(2.0, 0.99, "success_rate")
	f.Add(0.0, 0.0, "p95_latency_secs")
	f.Add(0.0, 0.0, "unknown_metric")

	f.Fuzz(func(t *testing.T, current, target float64, metric string) {
		got := CalculateErrorBudget(current, target, metric)
		assert.False(t, math.
			IsNaN(
				got))
		assert.False(t, math.
			IsInf(
				got, 0))

	})
}

func FuzzSLOEvaluator_LatencyBudget(f *testing.F) {
	f.Add(0.5, 1.0)
	f.Add(0.0, 0.0)
	f.Add(3.0, 2.0)
	f.Add(-1.0, 1.0)
	f.Add(math.MaxFloat64, 1.0)
	f.Add(1.0, math.SmallestNonzeroFloat64)

	f.Fuzz(func(t *testing.T, current, target float64) {
		for _, metric := range []string{domain.SLOMetricP95LatencySecs, domain.SLOMetricP99LatencySecs} {
			got := CalculateErrorBudget(current, target, metric)
			assert.False(t, math.
				IsNaN(
					got))
			assert.False(t, math.
				IsInf(
					got, 0))

		}
	})
}

func FuzzAnomalyMonitor_ZScore(f *testing.F) {
	f.Add(int64(100), int64(200), int64(300))
	f.Add(int64(0), int64(0), int64(0))
	f.Add(int64(-100), int64(0), int64(100))
	f.Add(int64(math.MaxInt64), int64(0), int64(1))
	f.Add(int64(1), int64(1), int64(1))

	f.Fuzz(func(t *testing.T, a, b, c int64) {
		// Build a mock anomaly monitor with fuzzed spending data.
		s := &mockAnomalyMonitorStore{
			listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
				return []string{"org-fuzz"}, nil
			},
			getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
				return &billing.OrgSubscription{OrgID: "org-fuzz", PlanTier: "pro"}, nil
			},
			getOrgUsageForPeriodFn: func(_ context.Context, _ string, _, _ time.Time) ([]billing.UsageRecord, error) {
				return []billing.UsageRecord{
					{OrgID: "org-fuzz", ComputeCostMicro: a},
					{OrgID: "org-fuzz", ComputeCostMicro: b},
					{OrgID: "org-fuzz", ComputeCostMicro: c},
				}, nil
			},
		}

		am := NewAnomalyMonitor(s, time.Minute)
		// Must not panic on any combination of spending values.
		am.check(context.Background())
	})
}

func FuzzCronScheduler_NextSchedule(f *testing.F) {
	f.Add("* * * * *")
	f.Add("*/5 * * * *")
	f.Add("")
	f.Add("0 0 31 2 *")
	f.Add("' OR 1=1 --")
	f.Add("\u00e9\u00e8\u00ea\u4e16\u754c")
	f.Add("59 23 31 12 7")
	f.Add("-1 -1 -1 -1 -1")

	f.Fuzz(func(t *testing.T, expr string) {
		ctx := context.Background()
		s := &mockCronStore{
			listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
				return []domain.Job{
					{ID: "j-fuzz", ProjectID: "proj-1", Cron: expr},
				}, nil
			},
			listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
				return nil, nil
			},
		}

		cs := NewCronScheduler(ctx, s, &mockQueue{}, nil)
		// Must not panic. Error is acceptable for invalid expressions.
		_ = cs.LoadJobs(ctx)
	})
}

func FuzzUsageReportEmailer_HTML(f *testing.F) {
	f.Add("org-1", "pro", int64(100_000), 1, int64(50_000))
	f.Add("", "", int64(0), 0, int64(0))
	f.Add("<script>alert('xss')</script>", "free", int64(-1), -1, int64(-1))
	f.Add("org'; DROP TABLE--", "starter", int64(math.MaxInt64), 999, int64(math.MaxInt64))
	f.Add("\u00e9\u00e8\u00ea\u4e16\u754c\u00fc", "enterprise", int64(1), 0, int64(0))

	f.Fuzz(func(t *testing.T, orgID, planTier string, creditMicro int64, addonCount int, overageMicro int64) {
		periodStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		periodEnd := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

		// Must not panic on any input combination.
		html := buildUsageReportHTML(orgID, planTier, periodStart, periodEnd, creditMicro, addonCount, overageMicro)
		assert.True(t, utf8.
			ValidString(html))

	})
}

func FuzzStaleSubscription_DateComparison(f *testing.F) {
	f.Add(int64(0))
	f.Add(time.Now().Unix())
	f.Add(time.Now().Add(-48 * time.Hour).Unix())
	f.Add(time.Now().Add(48 * time.Hour).Unix())
	f.Add(int64(-62135596800)) // time.Time zero
	f.Add(int64(253402300799)) // year 9999

	f.Fuzz(func(t *testing.T, unixSec int64) {
		// Protect against extremely out-of-range timestamps that could cause
		// time.Unix to produce values that crash time.Since.
		if unixSec < -62135596800 || unixSec > 253402300799 {
			return
		}

		ts := time.Unix(unixSec, 0)
		subID := "stripe-sub-fuzz"
		s := &mockStaleSubStore{
			listFn: func(context.Context) ([]billing.OrgSubscription, error) {
				return []billing.OrgSubscription{
					{
						OrgID:                "org-fuzz",
						PlanTier:             "pro",
						StripeSubscriptionID: &subID,
						CurrentPeriodEnd:     &ts,
					},
				}, nil
			},
		}

		checker := NewStaleSubscriptionChecker(s, time.Hour)
		// Must not panic on any timestamp.
		checker.check(context.Background())
	})
}
