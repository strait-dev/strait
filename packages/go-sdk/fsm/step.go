package fsm

import "fmt"

// StepRunStatus represents the status of a workflow step run.
type StepRunStatus string

const (
	StepRunPending   StepRunStatus = "pending"
	StepRunWaiting   StepRunStatus = "waiting"
	StepRunRunning   StepRunStatus = "running"
	StepRunCompleted StepRunStatus = "completed"
	StepRunFailed    StepRunStatus = "failed"
	StepRunSkipped   StepRunStatus = "skipped"
	StepRunCanceled  StepRunStatus = "canceled"
)

// StepRunEvent represents an event that triggers a step run state transition.
type StepRunEvent string

const (
	StepRunEventWait     StepRunEvent = "WAIT"
	StepRunEventStart    StepRunEvent = "START"
	StepRunEventComplete StepRunEvent = "COMPLETE"
	StepRunEventFail     StepRunEvent = "FAIL"
	StepRunEventSkip     StepRunEvent = "SKIP"
	StepRunEventCancel   StepRunEvent = "CANCEL"
)

var stepRunTransitions = map[StepRunStatus]map[StepRunEvent]StepRunStatus{
	StepRunPending: {
		StepRunEventWait:     StepRunWaiting,
		StepRunEventStart:    StepRunRunning,
		StepRunEventSkip:     StepRunSkipped,
		StepRunEventCancel:   StepRunCanceled,
		StepRunEventComplete: StepRunCompleted,
	},
	StepRunWaiting: {
		StepRunEventStart:    StepRunRunning,
		StepRunEventSkip:     StepRunSkipped,
		StepRunEventCancel:   StepRunCanceled,
		StepRunEventComplete: StepRunCompleted,
	},
	StepRunRunning: {
		StepRunEventComplete: StepRunCompleted,
		StepRunEventFail:     StepRunFailed,
		StepRunEventCancel:   StepRunCanceled,
	},
}

var terminalStepRunStatuses = map[StepRunStatus]bool{
	StepRunCompleted: true,
	StepRunFailed:    true,
	StepRunSkipped:   true,
	StepRunCanceled:  true,
}

// CanTransitionStepRun checks whether a step run transition is valid.
func CanTransitionStepRun(from StepRunStatus, event StepRunEvent) bool {
	events, ok := stepRunTransitions[from]
	if !ok {
		return false
	}
	_, ok = events[event]
	return ok
}

// TransitionStepRun returns the next status after applying the event.
func TransitionStepRun(from StepRunStatus, event StepRunEvent) (StepRunStatus, error) {
	events, ok := stepRunTransitions[from]
	if !ok {
		return "", fmt.Errorf("invalid step run status %q", from)
	}
	next, ok := events[event]
	if !ok {
		return "", fmt.Errorf("invalid transition: %q + %q", from, event)
	}
	return next, nil
}

// IsTerminalStepRunStatus returns true if the status is a terminal (final) state.
func IsTerminalStepRunStatus(status StepRunStatus) bool {
	return terminalStepRunStatuses[status]
}
