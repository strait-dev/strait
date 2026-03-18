package domain

import (
	"testing"
)

func TestEndpointHealthScore_HealthLevel_Boundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score float64
		want  string
	}{
		{0, "unhealthy"},
		{15, "unhealthy"},
		{29.99, "unhealthy"},
		{30, "degraded"},
		{45, "degraded"},
		{60, "degraded"},
		{60.01, "healthy"},
		{80, "healthy"},
		{100, "healthy"},
	}

	for _, tt := range tests {
		h := &EndpointHealthScore{HealthScore: tt.score}
		if got := h.HealthLevel(); got != tt.want {
			t.Errorf("HealthLevel(%.2f) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestTriggerJobCompletion_Constant(t *testing.T) {
	t.Parallel()
	if TriggerJobCompletion != "job_completion" {
		t.Errorf("TriggerJobCompletion = %q, want %q", TriggerJobCompletion, "job_completion")
	}
}

func TestSLOMetricConstants_Values(t *testing.T) {
	t.Parallel()
	if SLOMetricSuccessRate != "success_rate" {
		t.Errorf("SLOMetricSuccessRate = %q", SLOMetricSuccessRate)
	}
	if SLOMetricP95LatencySecs != "p95_latency_secs" {
		t.Errorf("SLOMetricP95LatencySecs = %q", SLOMetricP95LatencySecs)
	}
	if SLOMetricP99LatencySecs != "p99_latency_secs" {
		t.Errorf("SLOMetricP99LatencySecs = %q", SLOMetricP99LatencySecs)
	}
}

func TestJobSLOStatus_NilEvaluation(t *testing.T) {
	t.Parallel()
	status := JobSLOStatus{
		JobSLO: JobSLO{
			ID:          "slo-1",
			Metric:      SLOMetricSuccessRate,
			Target:      0.99,
			WindowHours: 24,
		},
	}

	if status.CurrentValue != nil {
		t.Error("CurrentValue should be nil without evaluation")
	}
	if status.BudgetRemaining != nil {
		t.Error("BudgetRemaining should be nil without evaluation")
	}
}

func TestJobSLOStatus_WithEvaluation(t *testing.T) {
	t.Parallel()
	cv := 0.95
	br := 0.6
	status := JobSLOStatus{
		JobSLO: JobSLO{
			ID:     "slo-1",
			Metric: SLOMetricP95LatencySecs,
			Target: 2.0,
		},
		CurrentValue:    &cv,
		BudgetRemaining: &br,
	}

	if *status.CurrentValue != 0.95 {
		t.Errorf("CurrentValue = %v, want 0.95", *status.CurrentValue)
	}
	if *status.BudgetRemaining != 0.6 {
		t.Errorf("BudgetRemaining = %v, want 0.6", *status.BudgetRemaining)
	}
}

func TestEndpointHealthScore_DefaultValues(t *testing.T) {
	t.Parallel()
	h := &EndpointHealthScore{}
	if h.HealthScore != 0 {
		t.Errorf("default HealthScore = %v, want 0", h.HealthScore)
	}
	if h.HealthLevel() != "unhealthy" {
		t.Errorf("default HealthLevel = %q, want unhealthy", h.HealthLevel())
	}
}

func TestJobSLO_ValidWindowHours(t *testing.T) {
	t.Parallel()
	valid := []int{24, 168, 720}
	for _, w := range valid {
		slo := JobSLO{WindowHours: w}
		if slo.WindowHours != w {
			t.Errorf("WindowHours = %d, want %d", slo.WindowHours, w)
		}
	}
}
