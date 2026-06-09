package domain

var validWorkflowTransitions = map[WorkflowRunStatus][]WorkflowRunStatus{
	WfStatusPending:            {WfStatusRunning, WfStatusCanceled},
	WfStatusRunning:            {WfStatusPaused, WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled},
	WfStatusPaused:             {WfStatusRunning, WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled},
	WfStatusCompleted:          {},
	WfStatusFailed:             {WfStatusCompensating},
	WfStatusTimedOut:           {WfStatusCompensating},
	WfStatusCanceled:           {},
	WfStatusCompensating:       {WfStatusCompensated, WfStatusCompensationFailed, WfStatusCanceled},
	WfStatusCompensated:        {},
	WfStatusCompensationFailed: {},
}

func ValidateWorkflowTransition(from, to WorkflowRunStatus) error {
	switch from {
	case WfStatusPending:
		if to == WfStatusRunning || to == WfStatusCanceled {
			return nil
		}
	case WfStatusRunning:
		if to == WfStatusPaused || to == WfStatusCompleted || to == WfStatusFailed || to == WfStatusTimedOut || to == WfStatusCanceled {
			return nil
		}
	case WfStatusPaused:
		if to == WfStatusRunning || to == WfStatusCompleted || to == WfStatusFailed || to == WfStatusTimedOut || to == WfStatusCanceled {
			return nil
		}
	case WfStatusCompleted, WfStatusCanceled, WfStatusCompensated, WfStatusCompensationFailed:
	case WfStatusFailed, WfStatusTimedOut:
		if to == WfStatusCompensating {
			return nil
		}
	case WfStatusCompensating:
		if to == WfStatusCompensated || to == WfStatusCompensationFailed || to == WfStatusCanceled {
			return nil
		}
	default:
		return &UnknownStatusError{Status: RunStatus(from)}
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
	switch from {
	case StepPending:
		if to == StepWaiting || to == StepRunning || to == StepSkipped || to == StepCanceled || to == StepCompleted {
			return nil
		}
	case StepWaiting:
		if to == StepRunning || to == StepSkipped || to == StepCanceled || to == StepCompleted {
			return nil
		}
	case StepRunning:
		if to == StepCompleted || to == StepFailed || to == StepCanceled {
			return nil
		}
	case StepCompleted, StepFailed, StepSkipped, StepCanceled:
	default:
		return &UnknownStatusError{Status: RunStatus(from)}
	}
	return &TransitionError{From: RunStatus(from), To: RunStatus(to)}
}
