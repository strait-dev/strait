package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"
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

	if signals.endpointKey != endpointStateKey(job.ProjectID, job.EndpointURL) {
		t.Fatalf("endpointKey = %q, want %q", signals.endpointKey, endpointStateKey(job.ProjectID, job.EndpointURL))
	}
	if signals.endpointURL != job.EndpointURL {
		t.Fatalf("endpointURL = %q, want %q", signals.endpointURL, job.EndpointURL)
	}
	if !signals.recordCircuitSuccess {
		t.Fatal("expected endpoint success to be recorded")
	}
	if signals.result.EndpointURL != signals.endpointKey {
		t.Fatalf("result endpoint = %q, want %q", signals.result.EndpointURL, signals.endpointKey)
	}
	if !signals.result.Success {
		t.Fatal("result success = false, want true")
	}
	if signals.result.LatencyMs != 1500 {
		t.Fatalf("latency = %v, want 1500", signals.result.LatencyMs)
	}
	if signals.result.JobTimeoutMs != 30000 {
		t.Fatalf("timeout = %v, want 30000", signals.result.JobTimeoutMs)
	}
}

func TestSuccessfulDispatchSignals_SkipsCircuitSuccessWithoutEndpointOrFallback(t *testing.T) {
	t.Parallel()

	transition := successfulRunTransition{execDur: time.Second}

	withoutEndpoint := newSuccessfulDispatchSignals(&domain.Job{ProjectID: "project-1"}, transition, true)
	if withoutEndpoint.recordCircuitSuccess {
		t.Fatal("empty endpoint should not record circuit success")
	}

	withTx := newSuccessfulDispatchSignals(&domain.Job{
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
	}, transition, false)
	if withTx.recordCircuitSuccess {
		t.Fatal("transactional completion should not record fallback circuit success")
	}
}

func TestSuccessfulLatencyAnomaly_RecordsAboveDoubleP95(t *testing.T) {
	t.Parallel()

	transition := successfulRunTransition{
		started: true,
		execDur: 4500 * time.Millisecond,
	}
	stats := &orcstore.JobHealthStats{P95DurationSecs: 2}

	anomaly := newSuccessfulLatencyAnomaly(transition, stats)

	if !anomaly.record {
		t.Fatal("record = false, want true")
	}
	if anomaly.duration != 4500*time.Millisecond {
		t.Fatalf("duration = %s, want 4.5s", anomaly.duration)
	}
	if anomaly.p95 != 2*time.Second {
		t.Fatalf("p95 = %s, want 2s", anomaly.p95)
	}
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
			if anomaly.record {
				t.Fatal("record = true, want false")
			}
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

	if payload.endpointKey != endpointStateKey(job.ProjectID, job.EndpointURL) {
		t.Fatalf("endpointKey = %q, want %q", payload.endpointKey, endpointStateKey(job.ProjectID, job.EndpointURL))
	}
	if payload.endpointURL != job.EndpointURL {
		t.Fatalf("endpointURL = %q, want %q", payload.endpointURL, job.EndpointURL)
	}
	if payload.logName != "failure" {
		t.Fatalf("logName = %q, want failure", payload.logName)
	}
	if !payload.circuitFailedAt.Equal(failedAt) {
		t.Fatalf("circuitFailedAt = %s, want %s", payload.circuitFailedAt, failedAt)
	}
	if payload.result.EndpointURL != payload.endpointKey {
		t.Fatalf("result endpoint = %q, want %q", payload.result.EndpointURL, payload.endpointKey)
	}
	if payload.result.Success {
		t.Fatal("result success = true, want false")
	}
	if payload.result.TimedOut {
		t.Fatal("result timedOut = true, want false")
	}
	if payload.result.LatencyMs != 0 {
		t.Fatalf("latency = %v, want 0", payload.result.LatencyMs)
	}
	if payload.result.JobTimeoutMs != 30000 {
		t.Fatalf("timeout = %v, want 30000", payload.result.JobTimeoutMs)
	}
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

	if payload.logName != "timeout" {
		t.Fatalf("logName = %q, want timeout", payload.logName)
	}
	if !payload.result.TimedOut {
		t.Fatal("result timedOut = false, want true")
	}
	if payload.result.LatencyMs != 45000 {
		t.Fatalf("latency = %v, want 45000", payload.result.LatencyMs)
	}
	if payload.result.JobTimeoutMs != 45000 {
		t.Fatalf("timeout = %v, want 45000", payload.result.JobTimeoutMs)
	}
}

func TestDeepSecEndpointStateKeyScopesByProject(t *testing.T) {
	t.Parallel()

	endpoint := "https://shared.example/run"
	a := endpointStateKey("proj-a", endpoint)
	b := endpointStateKey("proj-b", endpoint)
	if a == b {
		t.Fatal("endpoint state keys for different projects must differ")
	}
	if strings.Contains(a, "\x00") || strings.Contains(b, "\x00") {
		t.Fatalf("endpoint state keys must be valid Postgres text: %q %q", a, b)
	}
	if strings.Contains(a, endpoint) || strings.Contains(b, endpoint) {
		t.Fatalf("project-scoped endpoint state keys must not store raw endpoint URL: %q %q", a, b)
	}
	if endpointStateKey("", endpoint) != endpoint {
		t.Fatal("empty project should preserve legacy endpoint key")
	}
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
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	if calls[0].to != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", calls[0].to)
	}
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
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	if calls[0].to != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", calls[0].to)
	}
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
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	if calls[0].to != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", calls[0].to)
	}
}
