package worker

import (
	"context"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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

func (e *Executor) recordSuccessfulDispatchSignals(ctx context.Context, job *domain.Job, transition successfulRunTransition) {
	signals := newSuccessfulDispatchSignals(job, transition, e.txPool == nil)
	if signals.recordCircuitSuccess && e.shouldRecordCircuitSuccess(signals.endpointKey, time.Now()) {
		if err := e.store.RecordEndpointCircuitSuccess(ctx, signals.endpointKey); err != nil {
			e.clearCircuitSuccessSample(signals.endpointKey)
			e.logger.Warn("failed to record circuit breaker success", "endpoint", httputil.RedactURLForLog(signals.endpointURL), "error", err)
		}
	}
	if _, hsErr := e.healthScorer.RecordResult(ctx, signals.result); hsErr != nil {
		e.logger.Warn("failed to record health score success", "endpoint", httputil.RedactURLForLog(signals.endpointURL), "error", hsErr)
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

func (e *Executor) recordSuccessfulLatencyAnomaly(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	transition successfulRunTransition,
	stats *store.JobHealthStats,
) {
	if !transition.started {
		return
	}
	if stats == nil {
		var statsErr error
		stats, statsErr = e.getJobHealthStats(ctx, job.ID, time.Now())
		if statsErr != nil {
			stats = nil
		}
	}
	anomaly := newSuccessfulLatencyAnomaly(transition, stats)
	if !anomaly.record {
		return
	}
	e.logger.Warn("latency anomaly detected",
		"run_id", run.ID, "job_id", run.JobID,
		"duration_ms", anomaly.duration.Milliseconds(), "p95_ms", anomaly.p95.Milliseconds())
	if e.metrics != nil {
		e.metrics.LatencyAnomalies.Add(ctx, 1,
			metric.WithAttributes(attribute.String("job_id", run.JobID)))
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

func (e *Executor) recordFailedDispatchSignals(ctx context.Context, job *domain.Job, kind failedDispatchSignalKind) {
	signals := newFailedDispatchSignalPayload(job, kind, time.Now().UTC())
	e.endpointGuardCache.invalidate(signals.endpointKey)

	if err := e.store.RecordEndpointCircuitFailure(ctx, signals.endpointKey, signals.circuitFailedAt, e.circuitThreshold, e.circuitOpenFor); err != nil {
		e.logger.Warn("failed to record circuit breaker "+signals.logName, "endpoint", httputil.RedactURLForLog(signals.endpointURL), "error", err)
	}
	e.clearCircuitSuccessSample(signals.endpointKey)
	if _, hsErr := e.healthScorer.RecordResult(ctx, signals.result); hsErr != nil {
		e.logger.Warn("failed to record health score "+signals.logName, "endpoint", httputil.RedactURLForLog(signals.endpointURL), "error", hsErr)
	}
}

func (e *Executor) shouldRecordCircuitSuccess(endpointKey string, now time.Time) bool {
	if e == nil || endpointKey == "" || e.circuitSuccessSampleInterval <= 0 {
		return true
	}
	e.circuitSuccessMu.Lock()
	defer e.circuitSuccessMu.Unlock()
	if e.lastCircuitSuccess == nil {
		e.lastCircuitSuccess = make(map[string]time.Time)
	}
	last, ok := e.lastCircuitSuccess[endpointKey]
	if ok && now.Sub(last) < e.circuitSuccessSampleInterval {
		return false
	}
	e.lastCircuitSuccess[endpointKey] = now
	return true
}

func (e *Executor) clearCircuitSuccessSample(endpointKey string) {
	if e == nil || endpointKey == "" || e.circuitSuccessSampleInterval <= 0 {
		return
	}
	e.circuitSuccessMu.Lock()
	defer e.circuitSuccessMu.Unlock()
	delete(e.lastCircuitSuccess, endpointKey)
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
