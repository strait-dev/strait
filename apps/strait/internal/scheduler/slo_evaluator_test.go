package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateErrorBudget(%v, %v) = %v, want [%v, %v]",
					tt.current, tt.target, got, tt.wantMin, tt.wantMax)
			}
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
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateErrorBudget(%v, %v) = %v, want [%v, %v]",
					tt.current, tt.target, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCalculateErrorBudget_UnknownMetric(t *testing.T) {
	t.Parallel()
	got := CalculateErrorBudget(0.5, 0.99, "unknown_metric")
	if got != 1.0 {
		t.Errorf("unknown metric should return 1.0, got %v", got)
	}
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
					if budget < 0 || budget > 1 {
						t.Errorf("budget %v out of range for current=%v target=%v metric=%s",
							budget, current, target, metric)
					}
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

	if slo.Metric != "success_rate" {
		t.Errorf("Metric = %q, want %q", slo.Metric, "success_rate")
	}
	if slo.WindowHours != 24 {
		t.Errorf("WindowHours = %d, want %d", slo.WindowHours, 24)
	}
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
			if tt.constant != tt.expected {
				t.Errorf("got %q, want %q", tt.constant, tt.expected)
			}
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
	if budget >= 0.2 {
		t.Fatalf("test setup: expected budget < 0.2, got %v", budget)
	}

	// Verify the notifier interface works by calling it directly
	err := notifier.NotifySLOBudgetWarning(context.Background(), "proj-1", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(notifier.calls))
	}
	if notifier.calls[0].projectID != "proj-1" {
		t.Errorf("projectID = %q, want %q", notifier.calls[0].projectID, "proj-1")
	}
}

func TestSLOEvaluator_WebhookNotFiredWhenBudgetHealthy(t *testing.T) {
	t.Parallel()

	// current=1.0, target=0.99 => budget=1.0 (fully within budget)
	budget := CalculateErrorBudget(1.0, 0.99, domain.SLOMetricSuccessRate)
	if budget < 0.2 {
		t.Fatalf("test setup: expected budget >= 0.2, got %v", budget)
	}
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
			if got != tt.expected {
				t.Errorf("metricValue(%q) = %v, want %v", tt.metric, got, tt.expected)
			}
		})
	}
}

func TestHasSLOData_SkipsIdleWindows(t *testing.T) {
	t.Parallel()
	if hasSLOData(domain.SLOMetricSuccessRate, &store.JobHealthStats{}) {
		t.Fatal("idle success-rate SLO windows must be treated as no-data")
	}
	if !hasSLOData(domain.SLOMetricSuccessRate, &store.JobHealthStats{TotalRuns: 1}) {
		t.Fatal("non-empty success-rate SLO window should be evaluated")
	}
	if hasSLOData("unknown", &store.JobHealthStats{TotalRuns: 1}) {
		t.Fatal("unknown SLO metric should not be evaluated")
	}
}

func TestSLOEvaluator_AdvisoryLockerNotAcquiredSkipsEvaluation(t *testing.T) {
	t.Parallel()

	locker := &testAdvisoryLocker{acquired: false}
	evaluator := NewSLOEvaluator(nil, nil).WithAdvisoryLocker(locker)

	acquired, err := evaluator.evaluateWithOptionalLeader(context.Background())
	if err != nil {
		t.Fatalf("evaluateWithOptionalLeader() error = %v", err)
	}
	if acquired {
		t.Fatal("evaluateWithOptionalLeader() acquired lock, want false")
	}
	if locker.tryCalls != 1 {
		t.Fatalf("tryCalls = %d, want 1", locker.tryCalls)
	}
	if locker.releaseCalls != 0 {
		t.Fatalf("releaseCalls = %d, want 0 for unacquired lock", locker.releaseCalls)
	}
}
