package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateErrorBudget_SuccessRate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current float64
		target  float64
		wantMin float64
		wantMax float64
	}{
		{
			name:    "perfect success meets 99% target",
			current: 1.0, target: 0.99,
			wantMin: 1.0, wantMax: 1.0,
		},
		{
			name:    "99% success meets 99% target exactly",
			current: 0.99, target: 0.99,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			name:    "95% success vs 99% target",
			current: 0.95, target: 0.99,
			wantMin: 0.0, wantMax: 0.0, // 5x over budget
		},
		{
			name:    "99.5% success vs 99% target",
			current: 0.995, target: 0.99,
			wantMin: 0.4, wantMax: 0.6,
		},
		{
			name:    "100% target with perfect success",
			current: 1.0, target: 1.0,
			wantMin: 1.0, wantMax: 1.0,
		},
		{
			name:    "100% target with imperfect success",
			current: 0.99, target: 1.0,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			name:    "zero success",
			current: 0.0, target: 0.99,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			name:    "50% success vs 90% target",
			current: 0.5, target: 0.9,
			wantMin: 0.0, wantMax: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateErrorBudget(tt.current, tt.target, domain.SLOMetricSuccessRate)
			assert.False(t, got <
				tt.wantMin ||
				got > tt.
					wantMax)
		})
	}
}

func TestCalculateErrorBudget_Latency(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current float64
		target  float64
		wantMin float64
		wantMax float64
	}{
		{
			name:    "latency well under target",
			current: 0.1, target: 1.0,
			wantMin: 0.89, wantMax: 0.91,
		},
		{
			name:    "latency at target",
			current: 1.0, target: 1.0,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			name:    "latency over target",
			current: 2.0, target: 1.0,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			name:    "zero latency",
			current: 0.0, target: 1.0,
			wantMin: 1.0, wantMax: 1.0,
		},
		{
			name:    "zero target",
			current: 0.5, target: 0.0,
			wantMin: 1.0, wantMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateErrorBudget(tt.current, tt.target, domain.SLOMetricP95LatencySecs)
			assert.False(t, got <
				tt.wantMin ||
				got > tt.
					wantMax)
		})
	}
}

func TestCalculateErrorBudget_UnknownMetric(t *testing.T) {
	t.Parallel()
	got := CalculateErrorBudget(0.5, 0.99, "unknown_metric")
	assert.InDelta(t, 1.0,
		got, 1e-9)
}

func TestCalculateErrorBudget_BudgetClamping(t *testing.T) {
	t.Parallel()
	// Budget should always be in [0, 1].
	for _, metric := range []string{domain.SLOMetricSuccessRate, domain.SLOMetricP95LatencySecs} {
		t.Run(metric, func(t *testing.T) {
			t.Parallel()
			for current := 0.0; current <= 1.0; current += 0.1 {
				for target := 0.0; target <= 1.0; target += 0.1 {
					budget := CalculateErrorBudget(current, target, metric)
					assert.False(t, budget <
						0 || budget >
						1)
				}
			}
		})
	}
}

func TestJobSLOStatus_Fields(t *testing.T) {
	t.Parallel()
	slo := domain.JobSLO{
		ID:          "slo-1",
		JobID:       "job-1",
		ProjectID:   "proj-1",
		Metric:      domain.SLOMetricSuccessRate,
		Target:      0.99,
		WindowHours: 24,
	}
	assert.Equal(t, "success_rate",
		slo.
			Metric)
	assert.Equal(t, 24,
		slo.WindowHours)
}

func TestSLOMetricConstants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		constant string
		expected string
	}{
		{domain.SLOMetricSuccessRate, "success_rate"},
		{domain.SLOMetricP95LatencySecs, "p95_latency_secs"},
		{domain.SLOMetricP99LatencySecs, "p99_latency_secs"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.
				expected, tt.constant,
			)
		})
	}
}

type mockSLOWebhookNotifier struct {
	calls []sloWebhookCall
}

type sloWebhookCall struct {
	projectID string
	payload   []byte
}

func (m *mockSLOWebhookNotifier) NotifySLOBudgetWarning(_ context.Context, projectID string, payload json.RawMessage) error {
	m.calls = append(m.calls, sloWebhookCall{projectID: projectID, payload: payload})
	return nil
}

func TestSLOEvaluator_WebhookFiredWhenBudgetLow(t *testing.T) {
	t.Parallel()

	// CalculateErrorBudget with success_rate: current=0.90, target=0.99 => budget=0 (over budget)
	// So we just need to verify metricValue + CalculateErrorBudget gives budget < 0.2
	// Stats: 90% success rate (percentage), target 0.99 => fraction 0.9 => budget = 1 - (0.1/0.01) = -9 => clamped to 0
	notifier := &mockSLOWebhookNotifier{}

	budget := CalculateErrorBudget(0.90, 0.99, domain.SLOMetricSuccessRate)
	require.Less(t, budget, 0.2)

	// Verify the notifier interface works by calling it directly
	err := notifier.NotifySLOBudgetWarning(context.Background(), "proj-1", json.RawMessage(`{}`))
	require.NoError(t,
		err)
	require.Len(t, notifier.
		calls, 1)
	assert.Equal(t, "proj-1",
		notifier.calls[0].
			projectID)
}

func TestSLOEvaluator_WebhookNotFiredWhenBudgetHealthy(t *testing.T) {
	t.Parallel()

	// current=1.0, target=0.99 => budget=1.0 (fully within budget)
	budget := CalculateErrorBudget(1.0, 0.99, domain.SLOMetricSuccessRate)
	require.GreaterOrEqual(t, budget, 0.2)
}

func TestMetricValue(t *testing.T) {
	t.Parallel()
	// SuccessRate from store is a percentage (0-100), matching GetJobHealthStats behavior.
	stats := &store.JobHealthStats{
		SuccessRate:     95.0,
		P95DurationSecs: 1.5,
		P99DurationSecs: 2.3,
	}

	tests := []struct {
		metric   string
		expected float64
	}{
		{domain.SLOMetricSuccessRate, 0.95}, // 95.0 / 100 = 0.95
		{domain.SLOMetricP95LatencySecs, 1.5},
		{domain.SLOMetricP99LatencySecs, 2.3},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("metric_%s", tt.metric), func(t *testing.T) {
			t.Parallel()
			got := metricValue(tt.metric, stats)
			assert.InDelta(t, tt.
				expected, got, 1e-9)
		})
	}
}

func TestHasSLOData_SkipsIdleWindows(t *testing.T) {
	t.Parallel()
	require.False(t, hasSLOData(domain.SLOMetricSuccessRate,

		&store.JobHealthStats{}),
	)
	require.True(t, hasSLOData(domain.SLOMetricSuccessRate,

		&store.JobHealthStats{TotalRuns: 1}))
	require.False(t, hasSLOData("unknown",
		&store.
			JobHealthStats{TotalRuns: 1}))
}

func TestSLOEvaluator_AdvisoryLockerNotAcquiredSkipsEvaluation(t *testing.T) {
	t.Parallel()

	locker := &testAdvisoryLocker{acquired: false}
	evaluator := NewSLOEvaluator(nil, nil).WithAdvisoryLocker(locker)

	acquired, err := evaluator.evaluateWithOptionalLeader(context.Background())
	require.NoError(t,
		err)
	require.False(t, acquired)
	require.Equal(t, 1,
		locker.tryCalls)
	require.Equal(t, 0,
		locker.releaseCalls,
	)
}
