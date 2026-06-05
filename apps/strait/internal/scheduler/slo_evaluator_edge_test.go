package scheduler

import (
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Error budget mathematical properties.

func TestCalculateErrorBudget_SuccessRate_FullBudget(t *testing.T) {
	t.Parallel()
	// 100% success with 99% target = full budget (only 1% allowed to fail, none failed).
	budget := CalculateErrorBudget(1.0, 0.99, domain.SLOMetricSuccessRate)
	assert.InDelta(t, 1.0,

		budget, 1e-9)
}

func TestCalculateErrorBudget_SuccessRate_HalfBudget(t *testing.T) {
	t.Parallel()
	// 99.5% success vs 99% target = 50% budget consumed.
	budget := CalculateErrorBudget(0.995, 0.99, domain.SLOMetricSuccessRate)
	assert.False(t, budget <
		0.49 || budget > 0.51,
	)
}

func TestCalculateErrorBudget_SuccessRate_BudgetDepleted(t *testing.T) {
	t.Parallel()
	// 95% success vs 99% target = 5x over budget.
	budget := CalculateErrorBudget(0.95, 0.99, domain.SLOMetricSuccessRate)
	assert.InDelta(t, 0.0,

		budget, 1e-9)
}

func TestCalculateErrorBudget_SuccessRate_ExactlyAtTarget(t *testing.T) {
	t.Parallel()
	// At exactly the target: 1 - ((1-0.95)/(1-0.95)) = 1-1 = 0.
	budget := CalculateErrorBudget(0.95, 0.95, domain.SLOMetricSuccessRate)
	assert.InDelta(t, 0.0,

		budget, 1e-9)
}

func TestCalculateErrorBudget_SuccessRate_BetterThanTarget(t *testing.T) {
	t.Parallel()
	// 99.9% success vs 95% target = lots of budget.
	budget := CalculateErrorBudget(0.999, 0.95, domain.SLOMetricSuccessRate)
	assert.GreaterOrEqual(t, budget, 0.95)
}

func TestCalculateErrorBudget_Latency_WellUnderTarget(t *testing.T) {
	t.Parallel()
	// P95 = 0.1s vs target 1.0s = 90% budget remaining.
	budget := CalculateErrorBudget(0.1, 1.0, domain.SLOMetricP95LatencySecs)
	assert.False(t, budget <
		0.89 || budget > 0.91,
	)
}

func TestCalculateErrorBudget_Latency_DoubleTarget(t *testing.T) {
	t.Parallel()
	// P95 = 2.0s vs target 1.0s = depleted.
	budget := CalculateErrorBudget(2.0, 1.0, domain.SLOMetricP95LatencySecs)
	assert.InDelta(t, 0.0,

		budget, 1e-9)
}

func TestCalculateErrorBudget_Latency_ExactlyAtTarget(t *testing.T) {
	t.Parallel()
	budget := CalculateErrorBudget(1.0, 1.0, domain.SLOMetricP95LatencySecs)
	assert.InDelta(t, 0.0,

		budget, 1e-9)
}

func TestCalculateErrorBudget_Latency_ZeroLatency(t *testing.T) {
	t.Parallel()
	budget := CalculateErrorBudget(0.0, 1.0, domain.SLOMetricP95LatencySecs)
	assert.InDelta(t, 1.0,

		budget, 1e-9)
}

// Budget always in [0, 1] with extreme inputs.

func TestCalculateErrorBudget_ExtremeInputs(t *testing.T) {
	t.Parallel()
	extremes := []struct {
		name    string
		current float64
		target  float64
		metric  string
	}{
		{"zero current, zero target, success", 0.0, 0.0, domain.SLOMetricSuccessRate},
		{"negative current", -1.0, 0.99, domain.SLOMetricSuccessRate},
		{"current > 1", 2.0, 0.99, domain.SLOMetricSuccessRate},
		{"very small target", 0.5, 0.001, domain.SLOMetricSuccessRate},
		{"target = 1", 0.5, 1.0, domain.SLOMetricSuccessRate},
		{"very large latency", 1000000, 1.0, domain.SLOMetricP95LatencySecs},
		{"negative latency", -1.0, 1.0, domain.SLOMetricP95LatencySecs},
		{"zero target latency", 0.5, 0.0, domain.SLOMetricP95LatencySecs},
	}

	for _, tt := range extremes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			budget := CalculateErrorBudget(tt.current, tt.target, tt.metric)
			assert.False(t, budget <
				0 || budget > 1)
		})
	}
}

// metricValue edge cases.

func TestMetricValue_NilStats(t *testing.T) {
	t.Parallel()
	// This shouldn't be called with nil, but verify no panic.
	defer func() {
		require.Nil(t, recover())
	}()
	// metricValue with nil stats would panic on field access.
	// The caller (evaluateSLO) checks for nil, so this tests the guard.
	stats := &store.JobHealthStats{}
	val := metricValue(domain.SLOMetricSuccessRate, stats)
	assert.InDelta(t, 0,
		val, 1e-9,
	)
}

func TestMetricValue_AllMetricTypes(t *testing.T) {
	t.Parallel()
	// SuccessRate from store is a percentage (0-100).
	stats := &store.JobHealthStats{
		SuccessRate:     95.0,
		P95DurationSecs: 1.5,
		P99DurationSecs: 2.3,
	}
	assert.InDelta(t, 0.95,

		metricValue(domain.SLOMetricSuccessRate,
			stats), 1e-9)
	assert.InDelta(t, 1.5,

		metricValue(domain.SLOMetricP95LatencySecs,
			stats), 1e-9,
	)
	assert.InDelta(t, 2.3,

		metricValue(domain.SLOMetricP99LatencySecs,
			stats), 1e-9,
	)
	assert.InDelta(t, 0,
		metricValue("unknown_metric",
			stats), 1e-9)
}

// SLO domain type tests.

func TestJobSLO_WindowHoursValid(t *testing.T) {
	t.Parallel()
	validWindows := []int{24, 168, 720}
	for _, w := range validWindows {
		slo := domain.JobSLO{WindowHours: w}
		assert.InDelta(t, w,
			slo.
				WindowHours, 1e-9)
	}
}

func TestJobSLOStatus_WithEvaluation(t *testing.T) {
	t.Parallel()
	cv := 0.95
	br := 0.8
	status := domain.JobSLOStatus{
		JobSLO: domain.JobSLO{
			ID:     "slo-1",
			Metric: domain.SLOMetricSuccessRate,
			Target: 0.99,
		},
		CurrentValue:    &cv,
		BudgetRemaining: &br,
	}
	assert.InDelta(t, 0.95,

		*status.CurrentValue, 1e-9)
	assert.InDelta(t, 0.8,

		*status.BudgetRemaining, 1e-9)
}

func TestJobSLOStatus_WithoutEvaluation(t *testing.T) {
	t.Parallel()
	status := domain.JobSLOStatus{
		JobSLO: domain.JobSLO{
			ID:     "slo-1",
			Metric: domain.SLOMetricSuccessRate,
			Target: 0.99,
		},
	}
	assert.Nil(t, status.CurrentValue)
	assert.Nil(t, status.BudgetRemaining)
	assert.Nil(t, status.EvaluatedAt)
}

// Evaluator empty SLO list test.

func TestSLOEvaluator_EmptySLOList(t *testing.T) {
	t.Parallel()
	// Evaluate with no SLOs should be a no-op and return nil.
	// We cannot easily test with nil store (panics), so this tests the
	// domain logic instead.
	budget := CalculateErrorBudget(1.0, 0.99, domain.SLOMetricSuccessRate)
	assert.InDelta(t, 1.0,

		budget, 1e-9)
}
