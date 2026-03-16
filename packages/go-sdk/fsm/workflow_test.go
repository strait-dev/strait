package fsm

import "testing"

func TestWorkflowRunTransitions_AllValid(t *testing.T) {
	tests := []struct {
		from  WorkflowRunStatus
		event WorkflowRunEvent
		to    WorkflowRunStatus
	}{
		// pending
		{WorkflowRunPending, WorkflowRunEventStart, WorkflowRunRunning},
		{WorkflowRunPending, WorkflowRunEventCancel, WorkflowRunCanceled},
		// running
		{WorkflowRunRunning, WorkflowRunEventPause, WorkflowRunPaused},
		{WorkflowRunRunning, WorkflowRunEventComplete, WorkflowRunCompleted},
		{WorkflowRunRunning, WorkflowRunEventFail, WorkflowRunFailed},
		{WorkflowRunRunning, WorkflowRunEventTimeout, WorkflowRunTimedOut},
		{WorkflowRunRunning, WorkflowRunEventCancel, WorkflowRunCanceled},
		// paused
		{WorkflowRunPaused, WorkflowRunEventResume, WorkflowRunRunning},
		{WorkflowRunPaused, WorkflowRunEventComplete, WorkflowRunCompleted},
		{WorkflowRunPaused, WorkflowRunEventFail, WorkflowRunFailed},
		{WorkflowRunPaused, WorkflowRunEventTimeout, WorkflowRunTimedOut},
		{WorkflowRunPaused, WorkflowRunEventCancel, WorkflowRunCanceled},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"+"+string(tt.event), func(t *testing.T) {
			if !CanTransitionWorkflowRun(tt.from, tt.event) {
				t.Errorf("expected CanTransitionWorkflowRun(%q, %q) = true", tt.from, tt.event)
			}
			got, err := TransitionWorkflowRun(tt.from, tt.event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.to {
				t.Errorf("TransitionWorkflowRun(%q, %q) = %q, want %q", tt.from, tt.event, got, tt.to)
			}
		})
	}
}

func TestWorkflowRunTransitions_InvalidFromTerminal(t *testing.T) {
	terminals := []WorkflowRunStatus{
		WorkflowRunCompleted, WorkflowRunFailed,
		WorkflowRunTimedOut, WorkflowRunCanceled,
	}

	for _, status := range terminals {
		if CanTransitionWorkflowRun(status, WorkflowRunEventStart) {
			t.Errorf("expected no transitions from terminal status %q", status)
		}
		_, err := TransitionWorkflowRun(status, WorkflowRunEventStart)
		if err == nil {
			t.Errorf("expected error for transition from terminal status %q", status)
		}
	}
}

func TestWorkflowRunTransitions_InvalidEvent(t *testing.T) {
	if CanTransitionWorkflowRun(WorkflowRunPending, WorkflowRunEventPause) {
		t.Error("expected PAUSE invalid from pending")
	}
}

func TestIsTerminalWorkflowRunStatus(t *testing.T) {
	terminals := []WorkflowRunStatus{
		WorkflowRunCompleted, WorkflowRunFailed,
		WorkflowRunTimedOut, WorkflowRunCanceled,
	}
	for _, s := range terminals {
		if !IsTerminalWorkflowRunStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}

	nonTerminals := []WorkflowRunStatus{
		WorkflowRunPending, WorkflowRunRunning, WorkflowRunPaused,
	}
	for _, s := range nonTerminals {
		if IsTerminalWorkflowRunStatus(s) {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}
