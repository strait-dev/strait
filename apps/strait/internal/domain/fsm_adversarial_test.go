package domain

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
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
				require.False(t,
					allowed &&
						err !=
							nil)
				require.False(t,
					!allowed &&
						err ==
							nil)
			})
		}
	}
}

func TestFSM_UnknownFromStatus(t *testing.T) {
	t.Parallel()

	err := ValidateTransition(RunStatus("bogus_status"), StatusQueued)
	require.Error(t,
		err)

	var unknownErr *UnknownStatusError
	require.ErrorAs(t,
		err, &unknownErr,
	)
	require.Equal(t,
		RunStatus("bogus_status"),
		unknownErr.
			Status)
}

func TestFSM_UnknownToStatus(t *testing.T) {
	t.Parallel()

	err := ValidateTransition(StatusQueued, RunStatus("nonexistent"))
	require.Error(t,
		err)

	var transErr *TransitionError
	require.ErrorAs(t,
		err, &transErr)
}

func TestFSM_EmptyStatus(t *testing.T) {
	t.Parallel()

	// Empty from should return unknown status error.
	err := ValidateTransition(RunStatus(""), RunStatus(""))
	require.Error(t,
		err)

	var unknownErr *UnknownStatusError
	require.ErrorAs(t,
		err, &unknownErr,
	)

	// Empty to with valid from should return transition error.
	err = ValidateTransition(StatusQueued, RunStatus(""))
	require.Error(t,
		err)
}

func TestFSM_NullByteStatus(t *testing.T) {
	t.Parallel()

	err := ValidateTransition(RunStatus("\x00"), StatusQueued)
	require.Error(t,
		err)

	err = ValidateTransition(StatusQueued, RunStatus("\x00"))
	require.Error(t,
		err)
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
				require.False(t,
					allowed &&
						err !=
							nil)
				require.False(t,
					!allowed &&
						err ==
							nil)
			})
		}
	}
}

func TestWorkflowFSM_UnknownStatus(t *testing.T) {
	t.Parallel()

	err := ValidateWorkflowTransition(WorkflowRunStatus("imaginary"), WfStatusRunning)
	require.Error(t,
		err)

	var unknownErr *UnknownStatusError
	require.ErrorAs(t,
		err, &unknownErr,
	)

	err = ValidateWorkflowTransition(WfStatusPending, WorkflowRunStatus("imaginary"))
	require.Error(t,
		err)

	var transErr *TransitionError
	require.ErrorAs(t,
		err, &transErr)
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
	require.NoError(t, err)

	err = ValidateScopes(nil)
	require.NoError(t, err)
}

func TestValidateScopes_DuplicateScopes(t *testing.T) {
	t.Parallel()

	// Duplicate valid scopes should still pass.
	err := ValidateScopes([]string{ScopeJobsRead, ScopeJobsRead, ScopeJobsRead})
	require.NoError(t, err)

	// Duplicate invalid scopes should fail.
	err = ValidateScopes([]string{"fake:scope", "fake:scope"})
	require.Error(t,
		err)
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
