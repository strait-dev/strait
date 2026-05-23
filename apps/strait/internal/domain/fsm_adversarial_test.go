package domain

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

// allRunStatuses enumerates every RunStatus constant for exhaustive testing.
var allRunStatuses = []RunStatus{
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
	StatusReplayStaged,
	StatusPaused,
}

// allWorkflowRunStatuses enumerates every WorkflowRunStatus constant.
var allWorkflowRunStatuses = []WorkflowRunStatus{
	WfStatusPending,
	WfStatusRunning,
	WfStatusPaused,
	WfStatusCompleted,
	WfStatusFailed,
	WfStatusTimedOut,
	WfStatusCanceled,
	WfStatusCompensating,
	WfStatusCompensated,
	WfStatusCompensationFailed,
	WfStatusContinued,
}

func TestFSM_ExhaustiveTransitionMatrix(t *testing.T) {
	t.Parallel()

	for _, from := range allRunStatuses {
		for _, to := range allRunStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				t.Parallel()
				err := ValidateTransition(from, to)

				// Determine whether this transition should be valid.
				allowed := false
				if targets, ok := validTransitions[from]; ok {
					allowed = slices.Contains(targets, to)
				}

				if allowed && err != nil {
					t.Fatalf("expected valid transition %s -> %s but got error: %v", from, to, err)
				}
				if !allowed && err == nil {
					t.Fatalf("expected invalid transition %s -> %s but got nil error", from, to)
				}
			})
		}
	}
}

func TestFSM_UnknownFromStatus(t *testing.T) {
	t.Parallel()

	err := ValidateTransition(RunStatus("bogus_status"), StatusQueued)
	if err == nil {
		t.Fatal("expected error for unknown from status, got nil")
	}
	var unknownErr *UnknownStatusError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownStatusError, got %T: %v", err, err)
	}
	if unknownErr.Status != RunStatus("bogus_status") {
		t.Fatalf("expected status bogus_status in error, got %s", unknownErr.Status)
	}
}

func TestFSM_UnknownToStatus(t *testing.T) {
	t.Parallel()

	err := ValidateTransition(StatusQueued, RunStatus("nonexistent"))
	if err == nil {
		t.Fatal("expected error for unknown to status, got nil")
	}
	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Fatalf("expected TransitionError, got %T: %v", err, err)
	}
}

func TestFSM_EmptyStatus(t *testing.T) {
	t.Parallel()

	// Empty from should return unknown status error.
	err := ValidateTransition(RunStatus(""), RunStatus(""))
	if err == nil {
		t.Fatal("expected error for empty from status, got nil")
	}
	var unknownErr *UnknownStatusError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownStatusError for empty from, got %T: %v", err, err)
	}

	// Empty to with valid from should return transition error.
	err = ValidateTransition(StatusQueued, RunStatus(""))
	if err == nil {
		t.Fatal("expected error for empty to status, got nil")
	}
}

func TestFSM_NullByteStatus(t *testing.T) {
	t.Parallel()

	err := ValidateTransition(RunStatus("\x00"), StatusQueued)
	if err == nil {
		t.Fatal("expected error for null byte from status, got nil")
	}

	err = ValidateTransition(StatusQueued, RunStatus("\x00"))
	if err == nil {
		t.Fatal("expected error for null byte to status, got nil")
	}
}

func FuzzFSMTransitionAdversarial(f *testing.F) {
	// Seed with known statuses.
	for _, s := range allRunStatuses {
		for _, s2 := range allRunStatuses {
			f.Add(string(s), string(s2))
		}
	}
	f.Add("", "")
	f.Add("\x00", "queued")
	f.Add("queued", "\x00")
	f.Add("QUEUED", "completed")

	f.Fuzz(func(t *testing.T, from, to string) {
		// Must not panic.
		_ = ValidateTransition(RunStatus(from), RunStatus(to))
	})
}

func TestWorkflowFSM_ExhaustiveMatrix(t *testing.T) {
	t.Parallel()

	for _, from := range allWorkflowRunStatuses {
		for _, to := range allWorkflowRunStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				t.Parallel()
				err := ValidateWorkflowTransition(from, to)

				// Determine expected result from the map.
				allowed := false
				if targets, ok := validWorkflowTransitions[from]; ok {
					allowed = slices.Contains(targets, to)
				}

				if allowed && err != nil {
					t.Fatalf("expected valid workflow transition %s -> %s but got error: %v", from, to, err)
				}
				if !allowed && err == nil {
					t.Fatalf("expected invalid workflow transition %s -> %s but got nil error", from, to)
				}
			})
		}
	}
}

func TestWorkflowFSM_UnknownStatus(t *testing.T) {
	t.Parallel()

	err := ValidateWorkflowTransition(WorkflowRunStatus("imaginary"), WfStatusRunning)
	if err == nil {
		t.Fatal("expected error for unknown workflow from status, got nil")
	}
	var unknownErr *UnknownStatusError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected UnknownStatusError, got %T: %v", err, err)
	}

	err = ValidateWorkflowTransition(WfStatusPending, WorkflowRunStatus("imaginary"))
	if err == nil {
		t.Fatal("expected error for invalid workflow to status, got nil")
	}
	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Fatalf("expected TransitionError, got %T: %v", err, err)
	}
}

func FuzzWorkflowFSMTransition(f *testing.F) {
	for _, s := range allWorkflowRunStatuses {
		for _, s2 := range allWorkflowRunStatuses {
			f.Add(string(s), string(s2))
		}
	}
	f.Add("", "")
	f.Add("bogus", "running")

	f.Fuzz(func(t *testing.T, from, to string) {
		// Must not panic.
		_ = ValidateWorkflowTransition(WorkflowRunStatus(from), WorkflowRunStatus(to))
	})
}

func TestValidateScopes_EmptySlice(t *testing.T) {
	t.Parallel()

	err := ValidateScopes([]string{})
	if err != nil {
		t.Fatalf("expected nil error for empty scopes, got: %v", err)
	}

	err = ValidateScopes(nil)
	if err != nil {
		t.Fatalf("expected nil error for nil scopes, got: %v", err)
	}
}

func TestValidateScopes_DuplicateScopes(t *testing.T) {
	t.Parallel()

	// Duplicate valid scopes should still pass.
	err := ValidateScopes([]string{ScopeJobsRead, ScopeJobsRead, ScopeJobsRead})
	if err != nil {
		t.Fatalf("expected nil error for duplicate valid scopes, got: %v", err)
	}

	// Duplicate invalid scopes should fail.
	err = ValidateScopes([]string{"fake:scope", "fake:scope"})
	if err == nil {
		t.Fatal("expected error for duplicate invalid scopes, got nil")
	}
}

func FuzzValidateScopesAdversarial(f *testing.F) {
	f.Add("jobs:read")
	f.Add("*")
	f.Add("")
	f.Add("\x00")
	f.Add("jobs:read,runs:write")
	f.Add("../../../etc/passwd")
	f.Add("a]b[c")

	f.Fuzz(func(t *testing.T, scope string) {
		// Must not panic.
		_ = ValidateScopes([]string{scope})
		_ = HasScope([]string{scope}, ScopeJobsRead)
	})
}
