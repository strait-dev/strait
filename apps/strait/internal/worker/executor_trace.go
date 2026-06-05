package worker

import (
	"strings"
	"time"

	"strait/internal/domain"
)

type executionTraceMode string

const (
	executionTraceOff    executionTraceMode = "off"
	executionTraceErrors executionTraceMode = "errors"
	executionTraceFull   executionTraceMode = "full"
)

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

func populateExecutionTraceRunTimings(execTrace *domain.ExecutionTrace, run *domain.JobRun, executeStart, traceEnd time.Time) {
	if execTrace == nil {
		return
	}
	execTrace.TotalMs = durationMillisecondsAtLeastOne(traceEnd.Sub(executeStart))
	if run == nil {
		return
	}
	queueWait := max(time.Duration(0), executeStart.Sub(run.CreatedAt))
	execTrace.QueueWaitMs = durationMillisecondsAtLeastOne(queueWait)
	if run.StartedAt != nil {
		dequeue := max(time.Duration(0), executeStart.Sub(*run.StartedAt))
		execTrace.DequeueMs = durationMillisecondsAtLeastOne(dequeue)
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
