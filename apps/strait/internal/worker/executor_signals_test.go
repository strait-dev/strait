package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuccessfulDispatchSignals_WithEndpoint(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		ID:          "job-1",
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
		TimeoutSecs: 30,
	}
	transition := successfulRunTransition{execDur: 1500 * time.Millisecond}

	signals := newSuccessfulDispatchSignals(job, transition, true)
	require.Equal(t,
		endpointStateKey(job.
			ProjectID,

			job.EndpointURL), signals.
			endpointKey)
	require.Equal(t,
		job.EndpointURL,
		signals.
			endpointURL,
	)
	require.True(t,
		signals.recordCircuitSuccess,
	)
	require.Equal(t,
		signals.endpointKey,

		signals.
			result.
			EndpointURL)
	require.True(t,
		signals.result.
			Success,
	)
	require.InDelta(t, 1500, signals.
		result.
		LatencyMs, 1e-9,
	)
	require.InDelta(t, 30000, signals.
		result.
		JobTimeoutMs, 1e-9,
	)
}

func TestSuccessfulDispatchSignals_SkipsCircuitSuccessWithoutEndpointOrFallback(t *testing.T) {
	t.Parallel()

	transition := successfulRunTransition{execDur: time.Second}

	withoutEndpoint := newSuccessfulDispatchSignals(&domain.Job{ProjectID: "project-1"}, transition, true)
	require.False(t,
		withoutEndpoint.
			recordCircuitSuccess,
	)

	withTx := newSuccessfulDispatchSignals(&domain.Job{
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
	}, transition, false)
	require.False(t,
		withTx.recordCircuitSuccess,
	)
}

func TestExecutorCircuitSuccessSampling(t *testing.T) {
	t.Parallel()

	exec := &Executor{circuitSuccessSampleInterval: time.Hour}
	now := time.Unix(100, 0)

	require.True(t, exec.shouldRecordCircuitSuccess("endpoint-a", now))
	require.False(t, exec.shouldRecordCircuitSuccess("endpoint-a", now.Add(time.Second)))
	require.True(t, exec.shouldRecordCircuitSuccess("endpoint-b", now.Add(time.Second)))
	require.True(t, exec.shouldRecordCircuitSuccess("endpoint-a", now.Add(time.Hour)))
}

func TestExecutorCircuitSuccessSampling_ClearResetsEndpoint(t *testing.T) {
	t.Parallel()

	exec := &Executor{circuitSuccessSampleInterval: time.Hour}
	now := time.Unix(100, 0)

	require.True(t, exec.shouldRecordCircuitSuccess("endpoint-a", now))
	require.False(t, exec.shouldRecordCircuitSuccess("endpoint-a", now.Add(time.Second)))
	exec.clearCircuitSuccessSample("endpoint-a")
	require.True(t, exec.shouldRecordCircuitSuccess("endpoint-a", now.Add(2*time.Second)))
}

func TestSuccessfulLatencyAnomaly_RecordsAboveDoubleP95(t *testing.T) {
	t.Parallel()

	transition := successfulRunTransition{
		started: true,
		execDur: 4500 * time.Millisecond,
	}
	stats := &orcstore.JobHealthStats{P95DurationSecs: 2}

	anomaly := newSuccessfulLatencyAnomaly(transition, stats)
	require.True(t,
		anomaly.record,
	)
	require.Equal(t,
		4500*time.
			Millisecond,
		anomaly.
			duration)
	require.Equal(t,
		2*time.Second,
		anomaly.
			p95)
}

func TestSuccessfulLatencyAnomaly_SkipsWithoutStartedStatsOrThreshold(t *testing.T) {
	t.Parallel()

	stats := &orcstore.JobHealthStats{P95DurationSecs: 2}
	tests := []struct {
		name       string
		transition successfulRunTransition
		stats      *orcstore.JobHealthStats
	}{
		{
			name:       "not started",
			transition: successfulRunTransition{started: false, execDur: 5 * time.Second},
			stats:      stats,
		},
		{
			name:       "no stats",
			transition: successfulRunTransition{started: true, execDur: 5 * time.Second},
			stats:      nil,
		},
		{
			name:       "zero p95",
			transition: successfulRunTransition{started: true, execDur: 5 * time.Second},
			stats:      &orcstore.JobHealthStats{},
		},
		{
			name:       "exactly double p95",
			transition: successfulRunTransition{started: true, execDur: 4 * time.Second},
			stats:      stats,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			anomaly := newSuccessfulLatencyAnomaly(tt.transition, tt.stats)
			require.False(t,
				anomaly.record,
			)
		})
	}
}

func TestFailedDispatchSignalPayload_Failure(t *testing.T) {
	t.Parallel()

	failedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	job := &domain.Job{
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
		TimeoutSecs: 30,
	}

	payload := newFailedDispatchSignalPayload(job, failedDispatchSignalFailure, failedAt)
	require.Equal(t,
		endpointStateKey(job.
			ProjectID,

			job.EndpointURL), payload.
			endpointKey)
	require.Equal(t,
		job.EndpointURL,
		payload.
			endpointURL,
	)
	require.Equal(t,
		"failure",
		payload.logName,
	)
	require.True(t,
		payload.circuitFailedAt.
			Equal(
				failedAt,
			))
	require.Equal(t,
		payload.endpointKey,

		payload.
			result.
			EndpointURL)
	require.False(t,
		payload.result.
			Success,
	)
	require.False(t,
		payload.result.
			TimedOut,
	)
	require.InDelta(t, 0, payload.
		result.LatencyMs, 1e-9,
	)
	require.InDelta(t, 30000, payload.
		result.
		JobTimeoutMs, 1e-9,
	)
}

func TestFailedDispatchSignalPayload_Timeout(t *testing.T) {
	t.Parallel()

	failedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	job := &domain.Job{
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
		TimeoutSecs: 45,
	}

	payload := newFailedDispatchSignalPayload(job, failedDispatchSignalTimeout, failedAt)
	require.Equal(t,
		"timeout",
		payload.logName,
	)
	require.True(t,
		payload.result.
			TimedOut,
	)
	require.InDelta(t, 45000, payload.
		result.
		LatencyMs, 1e-9,
	)
	require.InDelta(t, 45000, payload.
		result.
		JobTimeoutMs, 1e-9,
	)
}

func TestDeepSecEndpointStateKeyScopesByProject(t *testing.T) {
	t.Parallel()

	endpoint := "https://shared.example/run"
	a := endpointStateKey("proj-a", endpoint)
	b := endpointStateKey("proj-b", endpoint)
	require.NotEqual(t, b, a)
	require.False(t,
		strings.Contains(a, "\x00") ||

			strings.Contains(b, "\x00"))
	require.False(t,
		strings.Contains(a, endpoint) ||
			strings.Contains(b, endpoint))
	require.Equal(t,
		endpoint,
		endpointStateKey("",

			endpoint))
}

func TestHandleSuccess_LatencyAnomalyDetected(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return &orcstore.JobHealthStats{P95DurationSecs: 1.0}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	startedAt := time.Now().Add(-3 * time.Second)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", StartedAt: &startedAt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)
	assert.Equal(t,
		domain.StatusCompleted,

		calls[0].to)
}

func TestHandleSuccess_LatencyNormal(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return &orcstore.JobHealthStats{P95DurationSecs: 10.0}, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	startedAt := time.Now().Add(-500 * time.Millisecond)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", StartedAt: &startedAt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)
	assert.Equal(t,
		domain.StatusCompleted,

		calls[0].to)
}

func TestHandleSuccess_NoStatsAvailable(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}

	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})

	startedAt := time.Now().Add(-3 * time.Second)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", StartedAt: &startedAt}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)
	assert.Equal(t,
		domain.StatusCompleted,

		calls[0].to)
}
