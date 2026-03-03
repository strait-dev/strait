package domain

import "testing"

func TestValidateTransition_AllValidTransitions(t *testing.T) {
	for from, toStatuses := range validTransitions {
		for _, to := range toStatuses {
			if err := ValidateTransition(from, to); err != nil {
				t.Fatalf("expected valid transition %s -> %s, got error: %v", from, to, err)
			}
		}
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from RunStatus
		to   RunStatus
	}{
		{StatusCompleted, StatusExecuting},
		{StatusFailed, StatusExecuting},
		{StatusQueued, StatusCompleted},
		{StatusDelayed, StatusExecuting},
		{StatusExecuting, StatusDelayed},
		{StatusWaiting, StatusDequeued},
		{StatusCanceled, StatusQueued},
		{StatusExpired, StatusQueued},
		{StatusSystemFailed, StatusCompleted},
		{StatusTimedOut, StatusWaiting},
		{StatusDequeued, StatusCompleted},
		{RunStatus("unknown"), StatusQueued},
	}

	for _, tc := range tests {
		if err := ValidateTransition(tc.from, tc.to); err == nil {
			t.Fatalf("expected invalid transition %s -> %s to fail", tc.from, tc.to)
		}
	}
}

func TestTerminalStatesHaveNoValidTransitions(t *testing.T) {
	for _, status := range TerminalStatuses() {
		transitions, ok := validTransitions[status]
		if !ok {
			t.Fatalf("terminal status %s not found in validTransitions", status)
		}
		if len(transitions) != 0 {
			t.Fatalf("terminal status %s should not have transitions", status)
		}
	}
}

func TestRunStatusIsTerminal_AllStatuses(t *testing.T) {
	tests := []struct {
		status   RunStatus
		expected bool
	}{
		{StatusDelayed, false},
		{StatusQueued, false},
		{StatusDequeued, false},
		{StatusExecuting, false},
		{StatusWaiting, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusTimedOut, true},
		{StatusCrashed, true},
		{StatusSystemFailed, true},
		{StatusCanceled, true},
		{StatusExpired, true},
	}

	for _, tc := range tests {
		if got := tc.status.IsTerminal(); got != tc.expected {
			t.Fatalf("status %s IsTerminal() = %v, expected %v", tc.status, got, tc.expected)
		}
	}
}

func TestAllStatusesCoveredByTransitionsMap(t *testing.T) {
	allStatuses := []RunStatus{
		StatusDelayed,
		StatusQueued,
		StatusDequeued,
		StatusExecuting,
		StatusWaiting,
		StatusCompleted,
		StatusFailed,
		StatusTimedOut,
		StatusCrashed,
		StatusSystemFailed,
		StatusCanceled,
		StatusExpired,
	}

	for _, status := range allStatuses {
		if _, ok := validTransitions[status]; !ok {
			t.Fatalf("status %s is missing from validTransitions map", status)
		}
	}

	if len(validTransitions) != len(allStatuses) {
		t.Fatalf("validTransitions has %d statuses, expected %d", len(validTransitions), len(allStatuses))
	}
}
