package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, tt.want, h.HealthLevel())
	}
}

func TestTriggerJobCompletion_Constant(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		"job_completion",

		TriggerJobCompletion)

}

func TestSLOMetricConstants_Values(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		"success_rate",

		SLOMetricSuccessRate)
	assert.Equal(t,
		"p95_latency_secs",

		SLOMetricP95LatencySecs)
	assert.Equal(t,
		"p99_latency_secs",

		SLOMetricP99LatencySecs)

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
	assert.Nil(t, status.CurrentValue)
	assert.Nil(t, status.BudgetRemaining)

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
	assert.Equal(t,
		0.95,

		*status.CurrentValue)
	assert.Equal(t,
		0.6,

		*status.BudgetRemaining)

}

func TestEndpointHealthScore_DefaultValues(t *testing.T) {
	t.Parallel()
	h := &EndpointHealthScore{}
	assert.Equal(t,
		float64(0),

		h.HealthScore)
	assert.Equal(t,
		"unhealthy",

		h.HealthLevel())

}

func TestJobSLO_ValidWindowHours(t *testing.T) {
	t.Parallel()
	valid := []int{24, 168, 720}
	for _, w := range valid {
		slo := JobSLO{WindowHours: w}
		assert.Equal(t,
			w,

			slo.WindowHours)

	}
}
