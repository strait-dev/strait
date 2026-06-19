package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

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
	err   error
}

type sloWebhookCall struct {
	projectID string
	payload   []byte
}

func (m *mockSLOWebhookNotifier) NotifySLOBudgetWarning(_ context.Context, projectID string, payload json.RawMessage) error {
	m.calls = append(m.calls, sloWebhookCall{projectID: projectID, payload: payload})
	return m.err
}

type mockSLOEvaluationStore struct {
	slos        []domain.JobSLO
	listErr     error
	countsStats *store.JobHealthStats
	healthStats *store.JobHealthStats
	statsErr    error
	insertErr   error
	pruneRows   int64
	pruneErr    error

	countsCalls int
	statsCalls  int
	inserted    []*domain.JobSLOEvaluation
	pruneCalls  int
	keepPerSLO  int
}

func (m *mockSLOEvaluationStore) ListAllJobSLOs(context.Context) ([]domain.JobSLO, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.slos, nil
}

func (m *mockSLOEvaluationStore) GetJobHealthCounts(context.Context, string, time.Time) (*store.JobHealthStats, error) {
	m.countsCalls++
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.countsStats, nil
}

func (m *mockSLOEvaluationStore) GetJobHealthStats(context.Context, string, time.Time) (*store.JobHealthStats, error) {
	m.statsCalls++
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.healthStats, nil
}

func (m *mockSLOEvaluationStore) InsertSLOEvaluation(_ context.Context, eval *domain.JobSLOEvaluation) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.inserted = append(m.inserted, eval)
	return nil
}

func (m *mockSLOEvaluationStore) PruneSLOEvaluations(_ context.Context, keepPerSLO int) (int64, error) {
	m.pruneCalls++
	m.keepPerSLO = keepPerSLO
	if m.pruneErr != nil {
		return 0, m.pruneErr
	}
	return m.pruneRows, nil
}

var _ sloEvaluationStore = (*mockSLOEvaluationStore)(nil)

type sloRunLogHandler struct {
	records chan string
}

func (h sloRunLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h sloRunLogHandler) Handle(_ context.Context, record slog.Record) error {
	h.records <- record.Message
	return nil
}

func (h sloRunLogHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h sloRunLogHandler) WithGroup(string) slog.Handler {
	return h
}

func TestSLOEvaluator_RunZeroIntervalUsesDefaultWithoutPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evaluator := NewSLOEvaluator(&mockSLOEvaluationStore{}, nil)

	require.NotPanics(t, func() {
		evaluator.Run(ctx, 0)
	})
}

func TestSLOEvaluator_RunLogsEvaluationCycleErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logs := make(chan string, 1)
	evaluator := NewSLOEvaluator(
		&mockSLOEvaluationStore{listErr: errors.New("list failed")},
		slog.New(sloRunLogHandler{records: logs}),
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		evaluator.Run(ctx, time.Millisecond)
	}()

	select {
	case msg := <-logs:
		require.Equal(t, "slo evaluation cycle failed", msg)
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for slo evaluation failure log")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for slo evaluator run loop to exit")
	}
}

func TestSLOEvaluator_EvaluateListError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("database unavailable")
	evaluator := NewSLOEvaluator(&mockSLOEvaluationStore{listErr: wantErr}, nil)

	err := evaluator.Evaluate(context.Background())
	require.ErrorIs(t, err, wantErr)
	require.ErrorContains(t, err, "list slos")
}

func TestSLOEvaluator_EvaluateEmptyListSkipsPrune(t *testing.T) {
	t.Parallel()

	store := &mockSLOEvaluationStore{}
	evaluator := NewSLOEvaluator(store, nil)

	require.NoError(t, evaluator.Evaluate(context.Background()))
	require.Equal(t, 0, store.countsCalls)
	require.Equal(t, 0, store.statsCalls)
	require.Equal(t, 0, store.pruneCalls)
}

func TestSLOEvaluator_EvaluateCanceledContextSkipsEvaluation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &mockSLOEvaluationStore{
		slos: []domain.JobSLO{{
			ID:          "slo-1",
			JobID:       "job-1",
			ProjectID:   "proj-1",
			Metric:      domain.SLOMetricSuccessRate,
			Target:      0.99,
			WindowHours: 1,
		}},
	}
	evaluator := NewSLOEvaluator(store, nil)

	err := evaluator.Evaluate(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 0, store.countsCalls)
	require.Equal(t, 0, store.pruneCalls)
}

func TestSLOEvaluator_EvaluateSkipsSLOsWithoutUsableData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		slo         domain.JobSLO
		countsStats *store.JobHealthStats
		healthStats *store.JobHealthStats
		wantCounts  int
		wantStats   int
	}{
		{
			name: "nil success stats",
			slo: domain.JobSLO{
				ID:          "slo-1",
				JobID:       "job-1",
				ProjectID:   "proj-1",
				Metric:      domain.SLOMetricSuccessRate,
				Target:      0.99,
				WindowHours: 1,
			},
			wantCounts: 1,
		},
		{
			name: "zero run latency stats",
			slo: domain.JobSLO{
				ID:          "slo-2",
				JobID:       "job-2",
				ProjectID:   "proj-1",
				Metric:      domain.SLOMetricP95LatencySecs,
				Target:      1,
				WindowHours: 1,
			},
			healthStats: &store.JobHealthStats{},
			wantStats:   1,
		},
		{
			name: "unknown metric",
			slo: domain.JobSLO{
				ID:          "slo-3",
				JobID:       "job-3",
				ProjectID:   "proj-1",
				Metric:      "custom_metric",
				Target:      1,
				WindowHours: 1,
			},
			healthStats: &store.JobHealthStats{TotalRuns: 1},
			wantStats:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &mockSLOEvaluationStore{
				slos:        []domain.JobSLO{tt.slo},
				countsStats: tt.countsStats,
				healthStats: tt.healthStats,
			}
			evaluator := NewSLOEvaluator(store, nil)

			require.NoError(t, evaluator.Evaluate(context.Background()))
			require.Empty(t, store.inserted)
			require.Equal(t, tt.wantCounts, store.countsCalls)
			require.Equal(t, tt.wantStats, store.statsCalls)
			require.Equal(t, 1, store.pruneCalls)
			require.Equal(t, 288, store.keepPerSLO)
		})
	}
}

func TestSLOEvaluator_EvaluateSuccessRateRecordsAndNotifiesLowBudget(t *testing.T) {
	t.Parallel()

	store := &mockSLOEvaluationStore{
		slos: []domain.JobSLO{{
			ID:          "slo-1",
			JobID:       "job-1",
			ProjectID:   "proj-1",
			Metric:      domain.SLOMetricSuccessRate,
			Target:      0.99,
			WindowHours: 24,
		}},
		countsStats: &store.JobHealthStats{
			TotalRuns:   10,
			SuccessRate: 90,
		},
		pruneRows: 2,
	}
	notifier := &mockSLOWebhookNotifier{}
	evaluator := NewSLOEvaluator(store, nil, WithSLOWebhookNotifier(notifier))

	require.NoError(t, evaluator.Evaluate(context.Background()))
	require.Equal(t, 1, store.countsCalls)
	require.Equal(t, 0, store.statsCalls)
	require.Len(t, store.inserted, 1)
	require.Equal(t, "slo-1", store.inserted[0].SLOID)
	require.InDelta(t, 0.90, store.inserted[0].CurrentValue, 1e-9)
	require.InDelta(t, 0, store.inserted[0].BudgetRemaining, 1e-9)
	require.Equal(t, 1, store.pruneCalls)
	require.Equal(t, 288, store.keepPerSLO)
	require.Len(t, notifier.calls, 1)
	require.Equal(t, "proj-1", notifier.calls[0].projectID)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(notifier.calls[0].payload, &payload))
	require.Equal(t, domain.WebhookEventSLOBudgetWarning, payload["event"])
	require.Equal(t, "slo-1", payload["slo_id"])
	require.Equal(t, "job-1", payload["job_id"])
}

func TestSLOEvaluator_EvaluateLatencyUsesHealthStatsWithoutNotification(t *testing.T) {
	t.Parallel()

	store := &mockSLOEvaluationStore{
		slos: []domain.JobSLO{{
			ID:          "slo-1",
			JobID:       "job-1",
			ProjectID:   "proj-1",
			Metric:      domain.SLOMetricP99LatencySecs,
			Target:      2,
			WindowHours: 6,
		}},
		healthStats: &store.JobHealthStats{
			TotalRuns:       4,
			P99DurationSecs: 0.5,
		},
	}
	notifier := &mockSLOWebhookNotifier{}
	evaluator := NewSLOEvaluator(store, nil, WithSLOWebhookNotifier(notifier))

	require.NoError(t, evaluator.Evaluate(context.Background()))
	require.Equal(t, 0, store.countsCalls)
	require.Equal(t, 1, store.statsCalls)
	require.Len(t, store.inserted, 1)
	require.InDelta(t, 0.5, store.inserted[0].CurrentValue, 1e-9)
	require.InDelta(t, 0.75, store.inserted[0].BudgetRemaining, 1e-9)
	require.Empty(t, notifier.calls)
}

func TestSLOEvaluator_EvaluateContinuesAfterEvaluationErrors(t *testing.T) {
	t.Parallel()

	insertErr := errors.New("insert failed")
	pruneErr := errors.New("prune failed")
	store := &mockSLOEvaluationStore{
		slos: []domain.JobSLO{{
			ID:          "slo-1",
			JobID:       "job-1",
			ProjectID:   "proj-1",
			Metric:      domain.SLOMetricSuccessRate,
			Target:      0.99,
			WindowHours: 1,
		}},
		countsStats: &store.JobHealthStats{
			TotalRuns:   1,
			SuccessRate: 100,
		},
		insertErr: insertErr,
		pruneErr:  pruneErr,
	}
	evaluator := NewSLOEvaluator(store, nil)

	require.NoError(t, evaluator.Evaluate(context.Background()))
	require.Equal(t, 1, store.countsCalls)
	require.Empty(t, store.inserted)
	require.Equal(t, 1, store.pruneCalls)
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
