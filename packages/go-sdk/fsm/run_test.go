package fsm

import "testing"

func TestRunTransitions_AllValid(t *testing.T) {
	tests := []struct {
		from  RunStatus
		event RunEvent
		to    RunStatus
	}{
		// delayed
		{RunDelayed, RunEventEnqueue, RunQueued},
		{RunDelayed, RunEventCancel, RunCanceled},
		{RunDelayed, RunEventExpire, RunExpired},
		// queued
		{RunQueued, RunEventDequeue, RunDequeued},
		{RunQueued, RunEventCancel, RunCanceled},
		{RunQueued, RunEventExpire, RunExpired},
		// dequeued
		{RunDequeued, RunEventExecute, RunExecuting},
		{RunDequeued, RunEventRequeue, RunQueued},
		{RunDequeued, RunEventCancel, RunCanceled},
		{RunDequeued, RunEventSystemFail, RunSystemFailed},
		// executing
		{RunExecuting, RunEventComplete, RunCompleted},
		{RunExecuting, RunEventFail, RunFailed},
		{RunExecuting, RunEventTimeout, RunTimedOut},
		{RunExecuting, RunEventCrash, RunCrashed},
		{RunExecuting, RunEventCancel, RunCanceled},
		{RunExecuting, RunEventWait, RunWaiting},
		{RunExecuting, RunEventRequeue, RunQueued},
		{RunExecuting, RunEventSystemFail, RunSystemFailed},
		{RunExecuting, RunEventDeadLetter, RunDeadLetter},
		// waiting
		{RunWaiting, RunEventExecute, RunExecuting},
		{RunWaiting, RunEventComplete, RunCompleted},
		{RunWaiting, RunEventFail, RunFailed},
		{RunWaiting, RunEventCancel, RunCanceled},
		{RunWaiting, RunEventTimeout, RunTimedOut},
		// dead_letter
		{RunDeadLetter, RunEventRequeue, RunQueued},
		{RunDeadLetter, RunEventReplay, RunReplayStaged},
		// replay_staged
		{RunReplayStaged, RunEventEnqueue, RunQueued},
		{RunReplayStaged, RunEventCancel, RunCanceled},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"+"+string(tt.event), func(t *testing.T) {
			if !CanTransitionRun(tt.from, tt.event) {
				t.Errorf("expected CanTransitionRun(%q, %q) = true", tt.from, tt.event)
			}
			got, err := TransitionRun(tt.from, tt.event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.to {
				t.Errorf("TransitionRun(%q, %q) = %q, want %q", tt.from, tt.event, got, tt.to)
			}
		})
	}
}

func TestRunTransitions_InvalidFromTerminal(t *testing.T) {
	terminals := []RunStatus{
		RunCompleted, RunFailed, RunTimedOut, RunCrashed,
		RunSystemFailed, RunCanceled, RunExpired,
	}

	for _, status := range terminals {
		if CanTransitionRun(status, RunEventEnqueue) {
			t.Errorf("expected no transitions from terminal status %q", status)
		}
		_, err := TransitionRun(status, RunEventEnqueue)
		if err == nil {
			t.Errorf("expected error for transition from terminal status %q", status)
		}
	}
}

func TestRunTransitions_InvalidEvent(t *testing.T) {
	if CanTransitionRun(RunDelayed, RunEventComplete) {
		t.Error("expected COMPLETE invalid from delayed")
	}
	_, err := TransitionRun(RunDelayed, RunEventComplete)
	if err == nil {
		t.Error("expected error for invalid event")
	}
}

func TestIsTerminalRunStatus(t *testing.T) {
	terminals := []RunStatus{
		RunCompleted, RunFailed, RunTimedOut, RunCrashed,
		RunSystemFailed, RunCanceled, RunExpired,
	}
	for _, s := range terminals {
		if !IsTerminalRunStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}

	nonTerminals := []RunStatus{
		RunDelayed, RunQueued, RunDequeued, RunExecuting,
		RunWaiting, RunDeadLetter, RunReplayStaged,
	}
	for _, s := range nonTerminals {
		if IsTerminalRunStatus(s) {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}
