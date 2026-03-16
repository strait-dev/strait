package worker

import (
	"context"
	"time"

	"strait/internal/domain"
)

// RunEventType identifies the type of lifecycle event.
type RunEventType string

const (
	EventCompleted    RunEventType = "completed"
	EventTimedOut     RunEventType = "timed_out"
	EventSnoozed      RunEventType = "snoozed"
	EventRetried      RunEventType = "retried"
	EventDeadLettered RunEventType = "dead_lettered"
	EventSystemFailed RunEventType = "system_failed"
)

// RunLifecycleEvent represents a run state transition emitted by the executor.
type RunLifecycleEvent struct {
	Type       RunEventType
	Run        *domain.JobRun
	Job        *domain.Job
	FromStatus domain.RunStatus
	ToStatus   domain.RunStatus
	ExecTrace  *domain.ExecutionTrace
	QueueWait  time.Duration
	ExecDur    time.Duration
	Attempt    int
}

// RunEventSubscriber processes lifecycle events. Must not block.
type RunEventSubscriber func(ctx context.Context, event RunLifecycleEvent)

// runEventEnvelope wraps an event with its originating context.
type runEventEnvelope struct {
	ctx   context.Context
	event RunLifecycleEvent
}
