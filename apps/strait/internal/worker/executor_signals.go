package worker

import (
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type successfulDispatchSignals struct {
	endpointKey          string
	endpointURL          string
	recordCircuitSuccess bool
	result               DispatchResult
}

func newSuccessfulDispatchSignals(job *domain.Job, transition successfulRunTransition, recordCircuitSuccess bool) successfulDispatchSignals {
	endpointKey := endpointStateKey(job.ProjectID, job.EndpointURL)
	return successfulDispatchSignals{
		endpointKey:          endpointKey,
		endpointURL:          job.EndpointURL,
		recordCircuitSuccess: recordCircuitSuccess && job.EndpointURL != "",
		result: DispatchResult{
			EndpointURL:  endpointKey,
			Success:      true,
			LatencyMs:    float64(transition.execDur.Milliseconds()),
			JobTimeoutMs: float64(job.TimeoutSecs * 1000),
		},
	}
}

type successfulLatencyAnomaly struct {
	record   bool
	duration time.Duration
	p95      time.Duration
}

func newSuccessfulLatencyAnomaly(transition successfulRunTransition, stats *store.JobHealthStats) successfulLatencyAnomaly {
	if !transition.started || stats == nil || stats.P95DurationSecs <= 0 {
		return successfulLatencyAnomaly{}
	}
	p95 := time.Duration(stats.P95DurationSecs * float64(time.Second))
	return successfulLatencyAnomaly{
		record:   transition.execDur > 2*p95,
		duration: transition.execDur,
		p95:      p95,
	}
}

type failedDispatchSignalKind int

const (
	failedDispatchSignalFailure failedDispatchSignalKind = iota
	failedDispatchSignalTimeout
)

func (k failedDispatchSignalKind) logName() string {
	switch k {
	case failedDispatchSignalTimeout:
		return "timeout"
	default:
		return "failure"
	}
}

func (k failedDispatchSignalKind) timedOut() bool {
	return k == failedDispatchSignalTimeout
}

func (k failedDispatchSignalKind) latencyMs(job *domain.Job) float64 {
	if k.timedOut() {
		return float64(job.TimeoutSecs * 1000)
	}
	return 0
}

type failedDispatchSignalPayload struct {
	endpointKey     string
	endpointURL     string
	logName         string
	circuitFailedAt time.Time
	result          DispatchResult
}

func newFailedDispatchSignalPayload(job *domain.Job, kind failedDispatchSignalKind, circuitFailedAt time.Time) failedDispatchSignalPayload {
	endpointKey := endpointStateKey(job.ProjectID, job.EndpointURL)
	return failedDispatchSignalPayload{
		endpointKey:     endpointKey,
		endpointURL:     job.EndpointURL,
		logName:         kind.logName(),
		circuitFailedAt: circuitFailedAt,
		result: DispatchResult{
			EndpointURL:  endpointKey,
			Success:      false,
			TimedOut:     kind.timedOut(),
			LatencyMs:    kind.latencyMs(job),
			JobTimeoutMs: float64(job.TimeoutSecs * 1000),
		},
	}
}
