package worker

import (
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
		QueueWait:  runStartedQueueWait(run),
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
		QueueWait:  runStartedQueueWait(run),
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
		QueueWait:  runStartedQueueWait(run),
	}
}

func newSystemFailedRunEvent(run *domain.JobRun, transition systemFailureTransition) RunLifecycleEvent {
	return RunLifecycleEvent{
		Type:       EventSystemFailed,
		Run:        run,
		FromStatus: transition.from,
		ToStatus:   transition.to,
		Attempt:    run.Attempt,
		QueueWait:  runStartedQueueWait(run),
	}
}
