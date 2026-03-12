package domain

import (
	"fmt"
	"testing"
)

func TestValidateTransition_AllValidTransitions(t *testing.T) {
	t.Parallel()
	for from, toStatuses := range validTransitions {
		for _, to := range toStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				if err := ValidateTransition(from, to); err != nil {
					t.Errorf("expected valid transition %s -> %s, got error: %v", from, to, err)
				}
			})
		}
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	t.Parallel()
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
		t.Run(fmt.Sprintf("%s->%s", tc.from, tc.to), func(t *testing.T) {
			if err := ValidateTransition(tc.from, tc.to); err == nil {
				t.Errorf("expected invalid transition %s -> %s to fail", tc.from, tc.to)
			}
		})
	}
}

func TestTerminalStatesHaveNoValidTransitions(t *testing.T) {
	t.Parallel()
	for _, status := range TerminalStatuses() {
		t.Run(string(status), func(t *testing.T) {
			transitions, ok := validTransitions[status]
			if !ok {
				t.Errorf("terminal status %s not found in validTransitions", status)
			}
			if len(transitions) != 0 {
				t.Errorf("terminal status %s transitions = %v, want []", status, transitions)
			}
		})
	}
}

func TestRunStatusIsTerminal_AllStatuses(t *testing.T) {
	t.Parallel()
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
		{StatusDeadLetter, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsTerminal(); got != tc.expected {
				t.Errorf("status %s IsTerminal() = %v, expected %v", tc.status, got, tc.expected)
			}
		})
	}
}

func TestAllStatusesCoveredByTransitionsMap(t *testing.T) {
	t.Parallel()
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
		StatusDeadLetter,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			if _, ok := validTransitions[status]; !ok {
				t.Errorf("status %s is missing from validTransitions map", status)
			}
		})
	}

	if len(validTransitions) != len(allStatuses) {
		t.Fatalf("validTransitions has %d statuses, expected %d", len(validTransitions), len(allStatuses))
	}
}

func TestRunStatusIsValid(t *testing.T) {
	t.Parallel()
	if !StatusQueued.IsValid() {
		t.Fatal("expected queued to be valid")
	}
	if RunStatus("not-valid").IsValid() {
		t.Fatal("expected arbitrary status to be invalid")
	}
}

func TestValidateTransition_DeadLetterTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		from    RunStatus
		to      RunStatus
		wantErr bool
	}{
		{name: "executing to dead_letter is valid", from: StatusExecuting, to: StatusDeadLetter},
		{name: "dead_letter to queued is valid", from: StatusDeadLetter, to: StatusQueued},
		{name: "dead_letter to completed is invalid", from: StatusDeadLetter, to: StatusCompleted, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTransition(tc.from, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("expected transition %s -> %s to fail", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %s -> %s to succeed, got %v", tc.from, tc.to, err)
			}
		})
	}
}

func FuzzFSMTransition(f *testing.F) {
	f.Add("queued", "dequeued")
	f.Add("executing", "completed")
	f.Add("executing", "failed")
	f.Add("nonsense", "also_nonsense")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, from, to string) {
		_ = ValidateTransition(RunStatus(from), RunStatus(to))
	})
}

func BenchmarkFSMTransition(b *testing.B) {
	for b.Loop() {
		_ = ValidateTransition(StatusExecuting, StatusCompleted)
	}
}
