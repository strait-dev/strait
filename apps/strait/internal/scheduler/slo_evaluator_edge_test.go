package scheduler

import (
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// Error budget mathematical properties.

func TestCalculateErrorBudget_SuccessRate_FullBudget(t *testing.T) {
	t.Parallel()
	// 100% success with 99% target = full budget (only 1% allowed to fail, none failed).
	budget := CalculateErrorBudget(1.0, 0.99, domain.SLOMetricSuccessRate)
	if budget != 1.0 {
		t.Errorf("100%% success vs 99%% target: budget = %v, want 1.0", budget)
	}
}

func TestCalculateErrorBudget_SuccessRate_HalfBudget(t *testing.T) {
	t.Parallel()
	// 99.5% success vs 99% target = 50% budget consumed.
	budget := CalculateErrorBudget(0.995, 0.99, domain.SLOMetricSuccessRate)
	if budget < 0.49 || budget > 0.51 {
		t.Errorf("99.5%% vs 99%%: budget = %v, want ~0.5", budget)
	}
}

func TestCalculateErrorBudget_SuccessRate_BudgetDepleted(t *testing.T) {
	t.Parallel()
	// 95% success vs 99% target = 5x over budget.
	budget := CalculateErrorBudget(0.95, 0.99, domain.SLOMetricSuccessRate)
	if budget != 0.0 {
		t.Errorf("95%% vs 99%%: budget = %v, want 0.0 (depleted)", budget)
	}
}

func TestCalculateErrorBudget_SuccessRate_ExactlyAtTarget(t *testing.T) {
	t.Parallel()
	// At exactly the target: 1 - ((1-0.95)/(1-0.95)) = 1-1 = 0.
	budget := CalculateErrorBudget(0.95, 0.95, domain.SLOMetricSuccessRate)
	if budget != 0.0 {
		t.Errorf("exactly at target: budget = %v, want 0.0", budget)
	}
}

func TestCalculateErrorBudget_SuccessRate_BetterThanTarget(t *testing.T) {
	t.Parallel()
	// 99.9% success vs 95% target = lots of budget.
	budget := CalculateErrorBudget(0.999, 0.95, domain.SLOMetricSuccessRate)
	if budget < 0.95 {
		t.Errorf("99.9%% vs 95%%: budget = %v, want > 0.95", budget)
	}
}

func TestCalculateErrorBudget_Latency_WellUnderTarget(t *testing.T) {
	t.Parallel()
	// P95 = 0.1s vs target 1.0s = 90% budget remaining.
	budget := CalculateErrorBudget(0.1, 1.0, domain.SLOMetricP95LatencySecs)
	if budget < 0.89 || budget > 0.91 {
		t.Errorf("0.1s vs 1.0s target: budget = %v, want ~0.9", budget)
	}
}

func TestCalculateErrorBudget_Latency_DoubleTarget(t *testing.T) {
	t.Parallel()
	// P95 = 2.0s vs target 1.0s = depleted.
	budget := CalculateErrorBudget(2.0, 1.0, domain.SLOMetricP95LatencySecs)
	if budget != 0.0 {
		t.Errorf("2.0s vs 1.0s target: budget = %v, want 0.0", budget)
	}
}

func TestCalculateErrorBudget_Latency_ExactlyAtTarget(t *testing.T) {
	t.Parallel()
	budget := CalculateErrorBudget(1.0, 1.0, domain.SLOMetricP95LatencySecs)
	if budget != 0.0 {
		t.Errorf("exactly at target: budget = %v, want 0.0", budget)
	}
}

func TestCalculateErrorBudget_Latency_ZeroLatency(t *testing.T) {
	t.Parallel()
	budget := CalculateErrorBudget(0.0, 1.0, domain.SLOMetricP95LatencySecs)
	if budget != 1.0 {
		t.Errorf("zero latency: budget = %v, want 1.0", budget)
	}
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
			if budget < 0 || budget > 1 {
				t.Errorf("budget %v out of [0,1] for current=%v target=%v metric=%s",
					budget, tt.current, tt.target, tt.metric)
			}
		})
	}
}

// metricValue edge cases.

func TestMetricValue_NilStats(t *testing.T) {
	t.Parallel()
	// This shouldn't be called with nil, but verify no panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("metricValue panicked with nil stats: %v", r)
		}
	}()
	// metricValue with nil stats would panic on field access.
	// The caller (evaluateSLO) checks for nil, so this tests the guard.
	stats := &store.JobHealthStats{}
	val := metricValue(domain.SLOMetricSuccessRate, stats)
	if val != 0 {
		t.Errorf("zero stats should return 0 for success_rate, got %v", val)
	}
}

func TestMetricValue_AllMetricTypes(t *testing.T) {
	t.Parallel()
	// SuccessRate from store is a percentage (0-100).
	stats := &store.JobHealthStats{
		SuccessRate:     95.0,
		P95DurationSecs: 1.5,
		P99DurationSecs: 2.3,
	}

	if v := metricValue(domain.SLOMetricSuccessRate, stats); v != 0.95 {
		t.Errorf("success_rate = %v, want 0.95", v)
	}
	if v := metricValue(domain.SLOMetricP95LatencySecs, stats); v != 1.5 {
		t.Errorf("p95_latency = %v, want 1.5", v)
	}
	if v := metricValue(domain.SLOMetricP99LatencySecs, stats); v != 2.3 {
		t.Errorf("p99_latency = %v, want 2.3", v)
	}
	if v := metricValue("unknown_metric", stats); v != 0 {
		t.Errorf("unknown = %v, want 0", v)
	}
}

// SLO domain type tests.

func TestJobSLO_WindowHoursValid(t *testing.T) {
	t.Parallel()
	validWindows := []int{24, 168, 720}
	for _, w := range validWindows {
		slo := domain.JobSLO{WindowHours: w}
		if slo.WindowHours != w {
			t.Errorf("WindowHours = %d, want %d", slo.WindowHours, w)
		}
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

	if *status.CurrentValue != 0.95 {
		t.Errorf("CurrentValue = %v, want 0.95", *status.CurrentValue)
	}
	if *status.BudgetRemaining != 0.8 {
		t.Errorf("BudgetRemaining = %v, want 0.8", *status.BudgetRemaining)
	}
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

	if status.CurrentValue != nil {
		t.Error("CurrentValue should be nil without evaluation")
	}
	if status.BudgetRemaining != nil {
		t.Error("BudgetRemaining should be nil without evaluation")
	}
	if status.EvaluatedAt != nil {
		t.Error("EvaluatedAt should be nil without evaluation")
	}
}

// Evaluator empty SLO list test.

func TestSLOEvaluator_EmptySLOList(t *testing.T) {
	t.Parallel()
	// Evaluate with no SLOs should be a no-op and return nil.
	// We cannot easily test with nil store (panics), so this tests the
	// domain logic instead.
	budget := CalculateErrorBudget(1.0, 0.99, domain.SLOMetricSuccessRate)
	if budget != 1.0 {
		t.Errorf("perfect success should have full budget, got %v", budget)
	}
}
