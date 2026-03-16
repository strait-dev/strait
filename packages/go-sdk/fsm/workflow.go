package fsm

import "fmt"

// WorkflowRunStatus represents the status of a workflow run.
type WorkflowRunStatus string

const (
	WorkflowRunPending   WorkflowRunStatus = "pending"
	WorkflowRunRunning   WorkflowRunStatus = "running"
	WorkflowRunPaused    WorkflowRunStatus = "paused"
	WorkflowRunCompleted WorkflowRunStatus = "completed"
	WorkflowRunFailed    WorkflowRunStatus = "failed"
	WorkflowRunTimedOut  WorkflowRunStatus = "timed_out"
	WorkflowRunCanceled  WorkflowRunStatus = "canceled"
)

// WorkflowRunEvent represents an event that triggers a workflow run state transition.
type WorkflowRunEvent string

const (
	WorkflowRunEventStart    WorkflowRunEvent = "START"
	WorkflowRunEventPause    WorkflowRunEvent = "PAUSE"
	WorkflowRunEventResume   WorkflowRunEvent = "RESUME"
	WorkflowRunEventComplete WorkflowRunEvent = "COMPLETE"
	WorkflowRunEventFail     WorkflowRunEvent = "FAIL"
	WorkflowRunEventTimeout  WorkflowRunEvent = "TIMEOUT"
	WorkflowRunEventCancel   WorkflowRunEvent = "CANCEL"
)

var workflowRunTransitions = map[WorkflowRunStatus]map[WorkflowRunEvent]WorkflowRunStatus{
	WorkflowRunPending: {
		WorkflowRunEventStart:  WorkflowRunRunning,
		WorkflowRunEventCancel: WorkflowRunCanceled,
	},
	WorkflowRunRunning: {
		WorkflowRunEventPause:    WorkflowRunPaused,
		WorkflowRunEventComplete: WorkflowRunCompleted,
		WorkflowRunEventFail:     WorkflowRunFailed,
		WorkflowRunEventTimeout:  WorkflowRunTimedOut,
		WorkflowRunEventCancel:   WorkflowRunCanceled,
	},
	WorkflowRunPaused: {
		WorkflowRunEventResume:   WorkflowRunRunning,
		WorkflowRunEventComplete: WorkflowRunCompleted,
		WorkflowRunEventFail:     WorkflowRunFailed,
		WorkflowRunEventTimeout:  WorkflowRunTimedOut,
		WorkflowRunEventCancel:   WorkflowRunCanceled,
	},
}

var terminalWorkflowRunStatuses = map[WorkflowRunStatus]bool{
	WorkflowRunCompleted: true,
	WorkflowRunFailed:    true,
	WorkflowRunTimedOut:  true,
	WorkflowRunCanceled:  true,
}

// CanTransitionWorkflowRun checks whether a workflow run transition is valid.
func CanTransitionWorkflowRun(from WorkflowRunStatus, event WorkflowRunEvent) bool {
	events, ok := workflowRunTransitions[from]
	if !ok {
		return false
	}
	_, ok = events[event]
	return ok
}

// TransitionWorkflowRun returns the next status after applying the event.
func TransitionWorkflowRun(from WorkflowRunStatus, event WorkflowRunEvent) (WorkflowRunStatus, error) {
	events, ok := workflowRunTransitions[from]
	if !ok {
		return "", fmt.Errorf("invalid workflow run status %q", from)
	}
	next, ok := events[event]
	if !ok {
		return "", fmt.Errorf("invalid transition: %q + %q", from, event)
	}
	return next, nil
}

// IsTerminalWorkflowRunStatus returns true if the status is a terminal (final) state.
func IsTerminalWorkflowRunStatus(status WorkflowRunStatus) bool {
	return terminalWorkflowRunStatuses[status]
}
