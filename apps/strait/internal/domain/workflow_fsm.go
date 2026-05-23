package domain

import "slices"

var validWorkflowTransitions = map[WorkflowRunStatus][]WorkflowRunStatus{
	WfStatusPending:            {WfStatusRunning, WfStatusCanceled},
	WfStatusRunning:            {WfStatusPaused, WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled, WfStatusContinued},
	WfStatusPaused:             {WfStatusRunning, WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled, WfStatusContinued},
	WfStatusCompleted:          {},
	WfStatusFailed:             {WfStatusCompensating},
	WfStatusTimedOut:           {WfStatusCompensating},
	WfStatusCanceled:           {},
	WfStatusCompensating:       {WfStatusCompensated, WfStatusCompensationFailed, WfStatusCanceled},
	WfStatusCompensated:        {},
	WfStatusCompensationFailed: {},
	WfStatusContinued:          {},
}

func ValidateWorkflowTransition(from, to WorkflowRunStatus) error {
	transitions, ok := validWorkflowTransitions[from]
	if !ok {
		return &UnknownStatusError{Status: RunStatus(from)}
	}
	if slices.Contains(transitions, to) {
		return nil
	}
	return &TransitionError{From: RunStatus(from), To: RunStatus(to)}
}

var validStepTransitions = map[StepRunStatus][]StepRunStatus{
	StepPending:   {StepWaiting, StepRunning, StepSkipped, StepCanceled, StepCompleted},
	StepWaiting:   {StepRunning, StepSkipped, StepCanceled, StepCompleted},
	StepRunning:   {StepCompleted, StepFailed, StepCanceled},
	StepCompleted: {},
	StepFailed:    {},
	StepSkipped:   {},
	StepCanceled:  {},
}

func ValidateStepTransition(from, to StepRunStatus) error {
	transitions, ok := validStepTransitions[from]
	if !ok {
		return &UnknownStatusError{Status: RunStatus(from)}
	}
	if slices.Contains(transitions, to) {
		return nil
	}
	return &TransitionError{From: RunStatus(from), To: RunStatus(to)}
}
