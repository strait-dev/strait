package worker

import (
	"time"

	"strait/internal/domain"
)

func newCompletedRunEvent(
	run *domain.JobRun,
	job *domain.Job,
	execTrace *domain.ExecutionTrace,
	transition successfulRunTransition,
) RunLifecycleEvent {
	return RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        run,
		Job:        job,
		FromStatus: domain.StatusExecuting,
		ToStatus:   transition.to,
		ExecTrace:  execTrace,
		ExecDur:    transition.execDur,
		Attempt:    run.Attempt,
		QueueWait:  queueWait(run),
	}
}

func newTerminalRunEvent(
	eventType RunEventType,
	run *domain.JobRun,
	job *domain.Job,
	to domain.RunStatus,
	execTrace *domain.ExecutionTrace,
) RunLifecycleEvent {
	return RunLifecycleEvent{
		Type:       eventType,
		Run:        run,
		Job:        job,
		FromStatus: domain.StatusExecuting,
		ToStatus:   to,
		ExecTrace:  execTrace,
		Attempt:    run.Attempt,
		QueueWait:  queueWait(run),
	}
}

func newRetriedRunEvent(run *domain.JobRun, job *domain.Job, execTrace *domain.ExecutionTrace) RunLifecycleEvent {
	return RunLifecycleEvent{
		Type:       EventRetried,
		Run:        run,
		Job:        job,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusQueued,
		ExecTrace:  execTrace,
		Attempt:    run.Attempt + 1,
		QueueWait:  queueWait(run),
	}
}

func newSystemFailedRunEvent(run *domain.JobRun, transition systemFailureTransition) RunLifecycleEvent {
	return RunLifecycleEvent{
		Type:       EventSystemFailed,
		Run:        run,
		FromStatus: transition.from,
		ToStatus:   transition.to,
		Attempt:    run.Attempt,
		QueueWait:  queueWait(run),
	}
}

// queueWait returns the duration a run spent queued (created_at to started_at).
func queueWait(run *domain.JobRun) time.Duration {
	if run == nil || run.CreatedAt.IsZero() {
		return 0
	}
	if run.StartedAt == nil {
		return 0
	}
	return run.StartedAt.Sub(run.CreatedAt)
}
