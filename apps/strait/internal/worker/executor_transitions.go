package worker

import (
	"encoding/json"
	"time"

	"strait/internal/domain"
)

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

// boostPriority adds boost to current priority, capping at 10 and
// guarding against integer overflow.
func boostPriority(current, boost int) int {
	boosted := current + boost
	if boosted < current { // integer overflow
		return 10
	}
	return min(boosted, 10)
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
