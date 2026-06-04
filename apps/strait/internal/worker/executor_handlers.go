package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type executionTraceMode string

const (
	executionTraceOff    executionTraceMode = "off"
	executionTraceErrors executionTraceMode = "errors"
	executionTraceFull   executionTraceMode = "full"
)

const executionTimedOutError = "execution timed out"

func normalizeExecutionTraceMode(mode string) executionTraceMode {
	switch executionTraceMode(strings.ToLower(strings.TrimSpace(mode))) {
	case executionTraceErrors:
		return executionTraceErrors
	case executionTraceFull:
		return executionTraceFull
	default:
		return executionTraceOff
	}
}

func (e *Executor) shouldPersistExecutionTrace(status domain.RunStatus, execTrace *domain.ExecutionTrace) bool {
	if execTrace == nil {
		return false
	}
	switch e.executionTraceMode {
	case executionTraceFull:
		return true
	case executionTraceErrors:
		return status != domain.StatusCompleted
	default:
		return false
	}
}

func (e *Executor) addExecutionTraceField(fields map[string]any, status domain.RunStatus, execTrace *domain.ExecutionTrace) {
	if e.shouldPersistExecutionTrace(status, execTrace) {
		fields["execution_trace"] = execTrace
	}
}

// recordRetryAttempt samples the attempt number each time a run is
// re-enqueued for retry. No-op if queue metrics were never initialised.
func recordRetryAttempt(ctx context.Context, attempt int) {
	qm, err := queue.Metrics()
	if err != nil || qm == nil || qm.RetryAttempts == nil {
		return
	}
	qm.RetryAttempts.Record(ctx, float64(attempt))
}

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) bool {
	return e.handleSuccessWithStats(ctx, run, job, result, nil, nil)
}

func (e *Executor) handleSuccessWithStats(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	result json.RawMessage,
	execTrace *domain.ExecutionTrace,
	stats *store.JobHealthStats,
) bool {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSuccess")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run completed", run, job, map[string]any{
		"to_status": string(domain.StatusCompleted),
	})

	transition := e.newSuccessfulRunTransition(run, result, execTrace, time.Now())
	run.FinishedAt = &transition.finished
	run.Status = transition.to
	err := e.completeRunWithWebhook(ctx, run, job, transition.to, transition.fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run completed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return false
	}
	e.recordSuccessfulDispatchSignals(ctx, job, transition)

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.emit(ctx, newCompletedRunEvent(run, job, execTrace, transition))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_complete workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTrigger(ctx, run, job, result)
	}

	e.recordSuccessfulLatencyAnomaly(ctx, run, job, transition, stats)
	return true
}

type successfulRunTransition struct {
	to       domain.RunStatus
	fields   map[string]any
	finished time.Time
	execDur  time.Duration
	started  bool
}

func (e *Executor) newSuccessfulRunTransition(
	run *domain.JobRun,
	result json.RawMessage,
	execTrace *domain.ExecutionTrace,
	finished time.Time,
) successfulRunTransition {
	fields := map[string]any{
		"finished_at": finished,
	}
	if len(result) > 0 {
		fields["result"] = result
	}
	e.addExecutionTraceField(fields, domain.StatusCompleted, execTrace)

	var execDur time.Duration
	var started bool
	if run.StartedAt != nil {
		started = true
		execDur = finished.Sub(*run.StartedAt)
	}

	return successfulRunTransition{
		to:       domain.StatusCompleted,
		fields:   fields,
		finished: finished,
		execDur:  execDur,
		started:  started,
	}
}

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
	if signals.recordCircuitSuccess {
		if err := e.store.RecordEndpointCircuitSuccess(ctx, signals.endpointKey); err != nil {
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

func classifyError(err error) string {
	if err == nil {
		return domain.ErrorClassUnknown
	}

	// Budget errors take highest priority.
	if isBudgetError(err) {
		return domain.ErrorClassBudget
	}

	// Deadline / timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.ErrorClassTimeout
	}

	// OOM signals.
	if isOOMError(err) {
		return domain.ErrorClassOOM
	}

	// Endpoint HTTP status classification.
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		switch {
		case endpointErr.StatusCode == http.StatusTooManyRequests:
			return domain.ErrorClassRateLimited
		case endpointErr.StatusCode == http.StatusUnauthorized || endpointErr.StatusCode == http.StatusForbidden:
			return domain.ErrorClassAuth
		case endpointErr.StatusCode >= http.StatusBadRequest && endpointErr.StatusCode < http.StatusInternalServerError:
			return domain.ErrorClassClient
		case endpointErr.StatusCode >= http.StatusInternalServerError:
			return domain.ErrorClassServer
		}
	}

	// Connection errors.
	if isConnectionError(err) {
		return domain.ErrorClassConnection
	}

	// Generic network errors.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return domain.ErrorClassTransient
	}

	// Context canceled (not deadline) is transient.
	if errors.Is(err, context.Canceled) {
		return domain.ErrorClassTransient
	}

	return domain.ErrorClassUnknown
}

func isOOMError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "out of memory") ||
		strings.Contains(msg, "OOM") ||
		strings.Contains(msg, "memory limit exceeded") ||
		strings.Contains(msg, "ENOMEM")
}

func isConnectionError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

func isBudgetError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "budget exceeded") ||
		strings.Contains(msg, "cost limit")
}

// errorHash returns a 16-char hex digest of the first 200 runes of an error
// message. Used for poison pill detection to identify identical errors across
// retry attempts without storing the full error string in metadata. Truncates
// by rune so multi-byte UTF-8 sequences are never split mid-character.
func errorHash(errMsg string) string {
	runes := []rune(errMsg)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	h := sha256.Sum256([]byte(string(runes)))
	return hex.EncodeToString(h[:8])
}

func errorHashForError(err error) string {
	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		return errorHash(fmt.Sprintf("endpoint returned %d: %s", endpointErr.StatusCode, endpointErr.Body))
	}
	return errorHash(err.Error())
}

// boostPriority adds boost to current priority, capping at 10 and
// guarding against integer overflow.
func boostPriority(current, boost int) int {
	boosted := current + boost
	if boosted < current { // integer overflow
		return 10
	}
	return min(boosted, 10)
}

func shouldRetryForClass(errClass string) bool {
	switch errClass {
	case domain.ErrorClassClient, domain.ErrorClassAuth, domain.ErrorClassBudget, domain.ErrorClassOOM:
		return false
	default:
		return true
	}
}

func shouldUseFallbackForClass(errClass string) bool {
	switch errClass {
	case domain.ErrorClassTransient, domain.ErrorClassRateLimited, domain.ErrorClassConnection, domain.ErrorClassTimeout:
		return true
	default:
		return false
	}
}

func retryStatusFields(run *domain.JobRun, job *domain.Job, errMsg, errClass string) map[string]any {
	fields := map[string]any{
		"attempt":     run.Attempt + 1,
		"error":       errMsg,
		"error_class": errClass,
		"started_at":  nil,
		"finished_at": nil,
	}
	if job.RetryPriorityBoost > 0 {
		fields["priority"] = boostPriority(run.Priority, job.RetryPriorityBoost)
	}
	return fields
}

func terminalStatusFields(finishedAt time.Time, errMsg, errClass string) map[string]any {
	return map[string]any{
		"finished_at": finishedAt,
		"error":       errMsg,
		"error_class": errClass,
	}
}

type retryRequeueLogMessages struct {
	scheduleFailure string
	updateFailure   string
	success         string
}

type timeoutRunTransition struct {
	retry   bool
	retryAt time.Time
	fields  map[string]any
}

func newTimeoutRunTransition(run *domain.JobRun, job *domain.Job, policy executionPolicy, finishedAt time.Time) timeoutRunTransition {
	if run.Attempt < policy.maxAttempts {
		return timeoutRunTransition{
			retry:   true,
			retryAt: NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs),
			fields:  retryStatusFields(run, job, executionTimedOutError, domain.ErrorClassTransient),
		}
	}
	return timeoutRunTransition{
		fields: terminalStatusFields(finishedAt, executionTimedOutError, domain.ErrorClassTransient),
	}
}

type failurePoisonPillDetection struct {
	hash      string
	count     int
	threshold int
}

type failureRunTransition struct {
	retry      bool
	retryAt    time.Time
	errMsg     string
	errClass   string
	fields     map[string]any
	poisonPill *failurePoisonPillDetection
}

func newFailureRunTransition(
	run *domain.JobRun,
	job *domain.Job,
	policy executionPolicy,
	err error,
	errMsg string,
	errClass string,
	finishedAt time.Time,
) failureRunTransition {
	shouldRetry := run.Attempt < policy.maxAttempts
	if shouldRetry && !shouldRetryForClass(errClass) {
		shouldRetry = false
	}

	var metadataModified bool
	var poisonPill *failurePoisonPillDetection
	if shouldRetry && job.PoisonPillThreshold != nil && *job.PoisonPillThreshold > 0 {
		hash := errorHashForError(err)
		prevHash := run.Metadata["_error_hash"]
		count := 1
		if prevHash == hash {
			if raw, ok := run.Metadata["_error_hash_count"]; ok {
				if n, parseErr := strconv.Atoi(raw); parseErr == nil {
					count = n + 1
				}
			}
		}
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["_error_hash"] = hash
		run.Metadata["_error_hash_count"] = strconv.Itoa(count)
		metadataModified = true

		threshold := *job.PoisonPillThreshold
		if count >= threshold {
			shouldRetry = false
			errMsg = fmt.Sprintf("poison pill detected (same error %d times): %s", count, errMsg)
			poisonPill = &failurePoisonPillDetection{
				hash:      hash,
				count:     count,
				threshold: threshold,
			}
		}
	}

	if shouldRetry {
		fields := retryStatusFields(run, job, errMsg, errClass)
		if metadataModified {
			fields["metadata"] = run.Metadata
		}
		return failureRunTransition{
			retry:    true,
			retryAt:  NextRetryAtWithPolicy(run.Attempt, policy.retryBackoff, policy.retryInitialSecs, policy.retryMaxSecs),
			errMsg:   errMsg,
			errClass: errClass,
			fields:   fields,
		}
	}

	fields := terminalStatusFields(finishedAt, errMsg, errClass)
	if metadataModified {
		fields["metadata"] = run.Metadata
	}
	return failureRunTransition{
		errMsg:     errMsg,
		errClass:   errClass,
		fields:     fields,
		poisonPill: poisonPill,
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

func (e *Executor) requeueRunForRetry(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	retryAt time.Time,
	fields map[string]any,
	execTrace *domain.ExecutionTrace,
	logs retryRequeueLogMessages,
) bool {
	// Side-table schedule write keeps the indexed job_runs.next_retry_at
	// column untouched so the requeue UPDATE stays HOT-eligible.
	if scheduleErr := e.store.ScheduleRetry(ctx, run.ID, retryAt, run.Attempt+1); scheduleErr != nil {
		e.logger.Error(logs.scheduleFailure,
			"run_id", run.ID, "job_id", run.JobID, "error", scheduleErr)
		return false
	}
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields)
	if err != nil {
		e.logger.Error(
			logs.updateFailure,
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return false
	}
	if logs.success != "" {
		e.logger.Info(
			logs.success,
			"run_id", run.ID,
			"job_id", run.JobID,
			"attempt", run.Attempt+1,
			"next_retry_at", retryAt,
		)
	}
	recordRetryAttempt(ctx, run.Attempt+1)
	e.emit(ctx, newRetriedRunEvent(run, job, execTrace))
	return true
}

func (e *Executor) recordFailedDispatchSignals(ctx context.Context, job *domain.Job, kind failedDispatchSignalKind) {
	signals := newFailedDispatchSignalPayload(job, kind, time.Now().UTC())

	if err := e.store.RecordEndpointCircuitFailure(ctx, signals.endpointKey, signals.circuitFailedAt, e.circuitThreshold, e.circuitOpenFor); err != nil {
		e.logger.Warn("failed to record circuit breaker "+signals.logName, "endpoint", httputil.RedactURLForLog(signals.endpointURL), "error", err)
	}
	if _, hsErr := e.healthScorer.RecordResult(ctx, signals.result); hsErr != nil {
		e.logger.Warn("failed to record health score "+signals.logName, "endpoint", httputil.RedactURLForLog(signals.endpointURL), "error", hsErr)
	}
}

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, err error, execTrace *domain.ExecutionTrace) bool {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleFailure")
	defer span.End()

	errMsg := err.Error()
	errClass := classifyError(err)
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run failed", run, job, map[string]any{
		"error_class":  errClass,
		"max_attempts": policy.maxAttempts,
	})
	e.recordFailedDispatchSignals(ctx, job, failedDispatchSignalFailure)

	e.logger.Warn(
		"run failed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"max_attempts", policy.maxAttempts,
		"error", errMsg,
		"error_class", errClass,
	)

	now := time.Now()
	transition := newFailureRunTransition(run, job, policy, err, errMsg, errClass, now)
	if transition.poisonPill != nil {
		e.logger.Warn("poison pill detected: consecutive same-error failures",
			"run_id", run.ID, "error_hash", transition.poisonPill.hash, "count", transition.poisonPill.count,
			"threshold", transition.poisonPill.threshold)
	}

	if transition.retry {
		addWorkerRunBreadcrumb(ctx, "worker.retry", "run retry scheduled", run, job, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": transition.retryAt.Format(time.RFC3339),
			"error_class":   errClass,
		})
		return e.requeueRunForRetry(ctx, run, job, transition.retryAt, transition.fields, execTrace, retryRequeueLogMessages{
			scheduleFailure: "failed to schedule retry",
			updateFailure:   "failed to re-enqueue run",
			success:         "run re-enqueued for retry",
		})
	}

	errMsg = transition.errMsg
	errClass = transition.errClass
	fields := transition.fields
	run.FinishedAt = &now
	targetStatus := domain.StatusDeadLetter
	e.addExecutionTraceField(fields, targetStatus, execTrace)
	run.Status = targetStatus

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, map[string]any{"error_class": errClass})
		scope.SetLevel(sentry.LevelWarning)
		scope.SetContext("failure", map[string]any{
			"error_message": errMsg,
			"error_class":   errClass,
			"max_attempts":  policy.maxAttempts,
			"final_status":  string(targetStatus),
		})
		scope.SetFingerprint([]string{"run_dead_lettered", run.JobID})
		sentry.CaptureMessage(fmt.Sprintf("run dead-lettered: %s", errMsg))
	})

	updateErr := e.completeRunWithWebhook(ctx, run, job, targetStatus, fields)
	if updateErr != nil {
		e.logger.Error(
			"failed to mark run terminal",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", updateErr,
		)
		return false
	}
	e.emit(ctx, newTerminalRunEvent(EventDeadLettered, run, job, targetStatus, execTrace))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, errMsg)
	}
	return true
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job, policy executionPolicy, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleTimeout")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run timed out", run, job, map[string]any{
		"timeout_secs": policy.timeoutSecs,
		"max_attempts": policy.maxAttempts,
	})
	e.recordFailedDispatchSignals(ctx, job, failedDispatchSignalTimeout)

	e.logger.Warn(
		"run timed out",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"timeout_secs", policy.timeoutSecs,
	)

	now := time.Now()
	transition := newTimeoutRunTransition(run, job, policy, now)
	if transition.retry {
		e.requeueRunForRetry(ctx, run, job, transition.retryAt, transition.fields, execTrace, retryRequeueLogMessages{
			scheduleFailure: "failed to schedule timeout retry",
			updateFailure:   "failed to re-enqueue timed out run",
		})
		return
	}

	fields := transition.fields
	run.FinishedAt = &now
	run.Status = domain.StatusTimedOut
	e.addExecutionTraceField(fields, domain.StatusTimedOut, execTrace)

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, nil)
		scope.SetLevel(sentry.LevelWarning)
		scope.SetContext("timeout", map[string]any{
			"timeout_secs": policy.timeoutSecs,
			"max_attempts": policy.maxAttempts,
		})
		scope.SetFingerprint([]string{"run_timed_out", run.JobID})
		sentry.CaptureMessage("run timed out after all retries")
	})

	err := e.completeRunWithWebhook(ctx, run, job, domain.StatusTimedOut, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run timed_out",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.emit(ctx, newTerminalRunEvent(EventTimedOut, run, job, domain.StatusTimedOut, execTrace))
	e.notifyWorkflowCallback(ctx, run)

	// Trigger on_failure job/workflow if configured.
	if e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, executionTimedOutError)
	}
}

// completeRunWithWebhook atomically updates run status and enqueues a webhook
// delivery within a single database transaction. If the job has no webhook URL
// or no txPool is configured, it falls back to a plain status update.
// The run must be in StatusExecuting when this is called.
func (e *Executor) completeRunWithWebhook(ctx context.Context, run *domain.JobRun, job *domain.Job, to domain.RunStatus, fields map[string]any) error {
	completion := newTerminalRunCompletion(run, job, to, fields)
	if e.txPool != nil {
		return store.WithTx(ctx, e.txPool, func(q *store.Queries) error {
			if err := q.UpdateRunStatus(ctx, run.ID, completion.from, completion.to, completion.fields); err != nil {
				return err
			}
			if completion.recordEndpointSuccess {
				if err := q.RecordEndpointCircuitSuccess(ctx, endpointStateKey(job.ProjectID, job.EndpointURL)); err != nil {
					return err
				}
			}
			if !completion.enqueueWebhook {
				return nil
			}
			_, err := q.EnqueueRunWebhook(ctx, job, completion.webhookRun, e.webhookMaxRetry)
			return err
		})
	}
	if completion.enqueueWebhook {
		e.logger.Warn("txPool not configured, webhook delivery skipped for completed run",
			"run_id", run.ID, "job_id", job.ID, "webhook_url", httputil.RedactURLForLog(job.WebhookURL))
	}
	return e.store.UpdateRunStatus(ctx, run.ID, completion.from, completion.to, completion.fields)
}

type terminalRunCompletion struct {
	from                  domain.RunStatus
	to                    domain.RunStatus
	fields                map[string]any
	webhookRun            *domain.JobRun
	recordEndpointSuccess bool
	enqueueWebhook        bool
}

func newTerminalRunCompletion(run *domain.JobRun, job *domain.Job, to domain.RunStatus, fields map[string]any) terminalRunCompletion {
	return terminalRunCompletion{
		from:                  domain.StatusExecuting,
		to:                    to,
		fields:                fields,
		webhookRun:            runForTerminalWebhook(run, to, fields),
		recordEndpointSuccess: to == domain.StatusCompleted && job.EndpointURL != "",
		enqueueWebhook:        job.WebhookURL != "",
	}
}

func runForTerminalWebhook(run *domain.JobRun, status domain.RunStatus, fields map[string]any) *domain.JobRun {
	webhookRun := *run
	webhookRun.Status = status
	if result, ok := fields["result"].(json.RawMessage); ok {
		webhookRun.Result = result
	}
	if errMsg, ok := fields["error"].(string); ok {
		webhookRun.Error = errMsg
	}
	if finishedAt, ok := fields["finished_at"].(time.Time); ok {
		webhookRun.FinishedAt = &finishedAt
	}
	return &webhookRun
}

type systemFailureTransition struct {
	from     domain.RunStatus
	to       domain.RunStatus
	fields   map[string]any
	finished time.Time
}

func newSystemFailureTransition(run *domain.JobRun, reason string, finished time.Time) systemFailureTransition {
	return systemFailureTransition{
		from:     run.Status,
		to:       domain.StatusSystemFailed,
		fields:   terminalStatusFields(finished, reason, domain.ErrorClassServer),
		finished: finished,
	}
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.HandleSystemFailure")
	defer span.End()
	addWorkerRunBreadcrumb(ctx, "worker.dispatch", "run system failure", run, nil, map[string]any{
		"error_class": domain.ErrorClassServer,
	})

	sentry.WithScope(func(scope *sentry.Scope) {
		e.applyWorkerSentryScope(scope, run, map[string]any{"error_class": domain.ErrorClassServer})
		scope.SetLevel(sentry.LevelError)
		scope.SetFingerprint([]string{"system_failure", reason})
		sentry.CaptureMessage(fmt.Sprintf("system failure: %s", reason))
	})

	transition := newSystemFailureTransition(run, reason, time.Now())
	err := e.store.UpdateRunStatus(ctx, run.ID, transition.from, transition.to, transition.fields)
	run.FinishedAt = &transition.finished
	if err != nil {
		e.logger.Error(
			"failed to mark system failure",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	run.Status = transition.to
	e.emit(ctx, newSystemFailedRunEvent(run, transition))
	e.notifyWorkflowCallback(ctx, run)
	// No webhook for system failures — job may not be available
}

// handleSystemFailureWithJob wraps handleSystemFailure and additionally fires
// on_failure triggers when the job is available. Some system failure paths
// (panic recovery, job-not-found) don't have the job object, so the base
// handleSystemFailure cannot require it.
func (e *Executor) handleSystemFailureWithJob(ctx context.Context, run *domain.JobRun, job *domain.Job, reason string) {
	e.handleSystemFailure(ctx, run, reason)
	if job != nil && e.onCompleteTrigger != nil {
		e.onCompleteTrigger.MaybeTriggerOnFailure(ctx, run, job, reason)
	}
}

func durationMillisecondsAtLeastOne(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}

func addWorkerRunBreadcrumb(ctx context.Context, category, message string, run *domain.JobRun, job *domain.Job, data map[string]any) {
	if run == nil {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["run_id"] = run.ID
	data["job_id"] = run.JobID
	data["project_id"] = run.ProjectID
	data["attempt"] = run.Attempt
	data["status"] = string(run.Status)
	data["execution_mode"] = string(run.ExecutionMode)
	if job != nil {
		data["job_version"] = job.Version
		data["environment_id"] = job.EnvironmentID
	}
	telemetry.AddSentryBreadcrumb(ctx, category, message, data)
}

func (e *Executor) applyWorkerSentryScope(scope *sentry.Scope, run *domain.JobRun, data map[string]any) {
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemWorker,
		Mode:      e.mode,
		Region:    e.defaultRegion,
		Version:   e.version,
	})
	if run != nil {
		telemetry.SetSentryTag(scope, telemetry.TagRunID, run.ID)
		telemetry.SetSentryTag(scope, telemetry.TagJobID, run.JobID)
		telemetry.SetSentryTag(scope, telemetry.TagProjectID, run.ProjectID)
		telemetry.SetSentryTag(scope, telemetry.TagAttempt, fmt.Sprintf("%d", run.Attempt))
		if run.CreatedBy != "" {
			actorType := workerActorType(run)
			telemetry.SetSentryTag(scope, telemetry.TagActorID, run.CreatedBy)
			telemetry.SetSentryTag(scope, telemetry.TagActorType, actorType)
			scope.SetUser(sentry.User{
				ID: run.CreatedBy,
				Data: map[string]string{
					"actor_type": actorType,
					"project_id": run.ProjectID,
				},
			})
		}
		requestContext := sentry.Context{
			"created_by":   run.CreatedBy,
			"triggered_by": run.TriggeredBy,
		}
		if requestID := run.Metadata[domain.RunMetadataSentryRequestID]; requestID != "" {
			telemetry.SetSentryTag(scope, telemetry.TagRequestID, requestID)
			requestContext["request_id"] = requestID
		}
		route := run.Metadata[domain.RunMetadataSentryRoute]
		if route == "" {
			route = "worker.dispatch"
		}
		telemetry.SetSentryTag(scope, telemetry.TagRoute, route)
		requestContext["route"] = route
		if actorType := run.Metadata[domain.RunMetadataSentryActorType]; actorType != "" {
			requestContext["actor_type"] = actorType
		}
		scope.SetContext("dispatch.request", requestContext)
		scope.SetContext("run", sentry.Context{
			"run_id":         run.ID,
			"job_id":         run.JobID,
			"project_id":     run.ProjectID,
			"attempt":        run.Attempt,
			"priority":       run.Priority,
			"execution_mode": string(run.ExecutionMode),
			"status":         string(run.Status),
		})
	}
	for key, val := range data {
		if tag, ok := telemetry.SentryTagFromString(key); ok {
			telemetry.SetSentryTag(scope, tag, fmt.Sprintf("%v", val))
		}
	}
}

func workerActorType(run *domain.JobRun) string {
	if run == nil {
		return ""
	}
	if actorType := run.Metadata[domain.RunMetadataSentryActorType]; actorType != "" {
		return actorType
	}
	switch {
	case strings.HasPrefix(run.CreatedBy, "apikey:"):
		return "api_key"
	case strings.HasPrefix(run.CreatedBy, "run:"):
		return "run_token"
	case strings.HasPrefix(run.CreatedBy, "sse:"):
		return "sse_token"
	case run.CreatedBy != "":
		return "user"
	default:
		return ""
	}
}
