// Package fsm provides table-driven finite state machines for run,
// workflow run, and step run lifecycle management.
package fsm

import "fmt"

// RunStatus represents the status of a job run.
type RunStatus string

const (
	RunDelayed      RunStatus = "delayed"
	RunQueued       RunStatus = "queued"
	RunDequeued     RunStatus = "dequeued"
	RunExecuting    RunStatus = "executing"
	RunWaiting      RunStatus = "waiting"
	RunCompleted    RunStatus = "completed"
	RunFailed       RunStatus = "failed"
	RunTimedOut     RunStatus = "timed_out"
	RunCrashed      RunStatus = "crashed"
	RunSystemFailed RunStatus = "system_failed"
	RunCanceled     RunStatus = "canceled"
	RunExpired      RunStatus = "expired"
	RunDeadLetter   RunStatus = "dead_letter"
	RunReplayStaged RunStatus = "replay_staged"
)

// RunEvent represents an event that triggers a run state transition.
type RunEvent string

const (
	RunEventEnqueue    RunEvent = "ENQUEUE"
	RunEventDequeue    RunEvent = "DEQUEUE"
	RunEventExecute    RunEvent = "EXECUTE"
	RunEventComplete   RunEvent = "COMPLETE"
	RunEventFail       RunEvent = "FAIL"
	RunEventTimeout    RunEvent = "TIMEOUT"
	RunEventCrash      RunEvent = "CRASH"
	RunEventSystemFail RunEvent = "SYSTEM_FAIL"
	RunEventCancel     RunEvent = "CANCEL"
	RunEventExpire     RunEvent = "EXPIRE"
	RunEventWait       RunEvent = "WAIT"
	RunEventRequeue    RunEvent = "REQUEUE"
	RunEventDeadLetter RunEvent = "DEAD_LETTER"
	RunEventReplay     RunEvent = "REPLAY"
)

var runTransitions = map[RunStatus]map[RunEvent]RunStatus{
	RunDelayed: {
		RunEventEnqueue: RunQueued,
		RunEventCancel:  RunCanceled,
		RunEventExpire:  RunExpired,
	},
	RunQueued: {
		RunEventDequeue: RunDequeued,
		RunEventCancel:  RunCanceled,
		RunEventExpire:  RunExpired,
	},
	RunDequeued: {
		RunEventExecute:    RunExecuting,
		RunEventRequeue:    RunQueued,
		RunEventCancel:     RunCanceled,
		RunEventSystemFail: RunSystemFailed,
	},
	RunExecuting: {
		RunEventComplete:   RunCompleted,
		RunEventFail:       RunFailed,
		RunEventTimeout:    RunTimedOut,
		RunEventCrash:      RunCrashed,
		RunEventCancel:     RunCanceled,
		RunEventWait:       RunWaiting,
		RunEventRequeue:    RunQueued,
		RunEventSystemFail: RunSystemFailed,
		RunEventDeadLetter: RunDeadLetter,
	},
	RunWaiting: {
		RunEventExecute:  RunExecuting,
		RunEventComplete: RunCompleted,
		RunEventFail:     RunFailed,
		RunEventCancel:   RunCanceled,
		RunEventTimeout:  RunTimedOut,
	},
	RunDeadLetter: {
		RunEventRequeue: RunQueued,
		RunEventReplay:  RunReplayStaged,
	},
	RunReplayStaged: {
		RunEventEnqueue: RunQueued,
		RunEventCancel:  RunCanceled,
	},
}

var terminalRunStatuses = map[RunStatus]bool{
	RunCompleted:    true,
	RunFailed:       true,
	RunTimedOut:     true,
	RunCrashed:      true,
	RunSystemFailed: true,
	RunCanceled:     true,
	RunExpired:      true,
}

// CanTransitionRun checks whether a transition from the given status via the
// given event is valid.
func CanTransitionRun(from RunStatus, event RunEvent) bool {
	events, ok := runTransitions[from]
	if !ok {
		return false
	}
	_, ok = events[event]
	return ok
}

// TransitionRun returns the next status after applying the event, or an error
// if the transition is invalid.
func TransitionRun(from RunStatus, event RunEvent) (RunStatus, error) {
	events, ok := runTransitions[from]
	if !ok {
		return "", fmt.Errorf("invalid run status %q", from)
	}
	next, ok := events[event]
	if !ok {
		return "", fmt.Errorf("invalid transition: %q + %q", from, event)
	}
	return next, nil
}

// IsTerminalRunStatus returns true if the status is a terminal (final) state.
func IsTerminalRunStatus(status RunStatus) bool {
	return terminalRunStatuses[status]
}
