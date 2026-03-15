package fsm

import "testing"

func TestStepRunTransitions_AllValid(t *testing.T) {
	tests := []struct {
		from  StepRunStatus
		event StepRunEvent
		to    StepRunStatus
	}{
		// pending
		{StepRunPending, StepRunEventWait, StepRunWaiting},
		{StepRunPending, StepRunEventStart, StepRunRunning},
		{StepRunPending, StepRunEventSkip, StepRunSkipped},
		{StepRunPending, StepRunEventCancel, StepRunCanceled},
		{StepRunPending, StepRunEventComplete, StepRunCompleted},
		// waiting
		{StepRunWaiting, StepRunEventStart, StepRunRunning},
		{StepRunWaiting, StepRunEventSkip, StepRunSkipped},
		{StepRunWaiting, StepRunEventCancel, StepRunCanceled},
		{StepRunWaiting, StepRunEventComplete, StepRunCompleted},
		// running
		{StepRunRunning, StepRunEventComplete, StepRunCompleted},
		{StepRunRunning, StepRunEventFail, StepRunFailed},
		{StepRunRunning, StepRunEventCancel, StepRunCanceled},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"+"+string(tt.event), func(t *testing.T) {
			if !CanTransitionStepRun(tt.from, tt.event) {
				t.Errorf("expected CanTransitionStepRun(%q, %q) = true", tt.from, tt.event)
			}
			got, err := TransitionStepRun(tt.from, tt.event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.to {
				t.Errorf("TransitionStepRun(%q, %q) = %q, want %q", tt.from, tt.event, got, tt.to)
			}
		})
	}
}

func TestStepRunTransitions_InvalidFromTerminal(t *testing.T) {
	terminals := []StepRunStatus{
		StepRunCompleted, StepRunFailed,
		StepRunSkipped, StepRunCanceled,
	}

	for _, status := range terminals {
		if CanTransitionStepRun(status, StepRunEventStart) {
			t.Errorf("expected no transitions from terminal status %q", status)
		}
		_, err := TransitionStepRun(status, StepRunEventStart)
		if err == nil {
			t.Errorf("expected error for transition from terminal status %q", status)
		}
	}
}

func TestStepRunTransitions_InvalidEvent(t *testing.T) {
	if CanTransitionStepRun(StepRunRunning, StepRunEventWait) {
		t.Error("expected WAIT invalid from running")
	}
}

func TestIsTerminalStepRunStatus(t *testing.T) {
	terminals := []StepRunStatus{
		StepRunCompleted, StepRunFailed,
		StepRunSkipped, StepRunCanceled,
	}
	for _, s := range terminals {
		if !IsTerminalStepRunStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}

	nonTerminals := []StepRunStatus{
		StepRunPending, StepRunWaiting, StepRunRunning,
	}
	for _, s := range nonTerminals {
		if IsTerminalStepRunStatus(s) {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}
