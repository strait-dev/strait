package worker

import (
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
