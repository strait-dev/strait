package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWorkflowTransition_AllValidTransitions(t *testing.T) {
	t.Parallel()
	for from, toStatuses := range validWorkflowTransitions {
		for _, to := range toStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				assert.NoError(
					t, ValidateWorkflowTransition(from, to))
			})
		}
	}
}

func TestValidateWorkflowTransition_InvalidTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from WorkflowRunStatus
		to   WorkflowRunStatus
	}{
		{WfStatusCompleted, WfStatusRunning},
		{WfStatusCompleted, WfStatusPending},
		{WfStatusFailed, WfStatusRunning},
		{WfStatusFailed, WfStatusCompleted},
		{WfStatusTimedOut, WfStatusRunning},
		{WfStatusCompensated, WfStatusRunning},
		{WfStatusCompensationFailed, WfStatusRunning},
		{WfStatusPending, WfStatusCompleted},
		{WfStatusRunning, WfStatusPending},
		{WfStatusPaused, WfStatusPending},
		{WfStatusCanceled, WfStatusRunning},
		{WfStatusCanceled, WfStatusPending},
		{WorkflowRunStatus("unknown"), WfStatusRunning},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s->%s", tc.from, tc.to), func(t *testing.T) {
			assert.Error(t,
				ValidateWorkflowTransition(tc.from, tc.to))
		})
	}
}

func TestValidateWorkflowTransition_UnknownStatus(t *testing.T) {
	t.Parallel()
	err := ValidateWorkflowTransition(WorkflowRunStatus("unknown"), WfStatusRunning)
	require.Error(t,
		err)

	var unknownErr *UnknownStatusError
	require.ErrorAs(t,
		err, &unknownErr)
}

func TestValidateWorkflowTransition_TerminalHaveNoTransitions(t *testing.T) {
	t.Parallel()
	// These statuses have zero outbound transitions.
	fullyTerminal := []WorkflowRunStatus{WfStatusCompleted, WfStatusCanceled, WfStatusCompensated, WfStatusCompensationFailed}
	for _, status := range fullyTerminal {
		t.Run(string(status), func(t *testing.T) {
			transitions, ok := validWorkflowTransitions[status]
			assert.True(t,
				ok)
			assert.Empty(t, transitions)
		})
	}
	// failed and timed_out can transition to compensating.
	for _, status := range []WorkflowRunStatus{WfStatusFailed, WfStatusTimedOut} {
		t.Run(string(status)+"_can_compensate", func(t *testing.T) {
			transitions := validWorkflowTransitions[status]
			assert.False(t,
				len(transitions) !=

					1 || transitions[0] != WfStatusCompensating)
		})
	}
}

func TestAllWorkflowStatusesCovered(t *testing.T) {
	t.Parallel()
	allStatuses := []WorkflowRunStatus{
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
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			assert.Contains(t, validWorkflowTransitions, status)
		})
	}
	require.Len(t,
		validWorkflowTransitions,

		len(allStatuses))
}

func TestWorkflowRunStatusIsTerminal_AllStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   WorkflowRunStatus
		expected bool
	}{
		{WfStatusPending, false},
		{WfStatusRunning, false},
		{WfStatusPaused, false},
		{WfStatusCompleted, true},
		{WfStatusFailed, true},
		{WfStatusTimedOut, true},
		{WfStatusCanceled, true},
		{WfStatusCompensating, false},
		{WfStatusCompensated, true},
		{WfStatusCompensationFailed, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.status.IsTerminal())
		})
	}
}

func TestWorkflowRunStatusIsValid(t *testing.T) {
	t.Parallel()
	valid := []WorkflowRunStatus{
		WfStatusRunning,
		WfStatusCompensating,
		WfStatusCompensated,
		WfStatusCompensationFailed,
	}
	for _, status := range valid {
		require.True(t,
			status.
				IsValid())
	}
	require.False(t,
		WorkflowRunStatus(
			"not-valid",
		).IsValid())
}

func TestValidateWorkflowTransition_ErrorTypes(t *testing.T) {
	t.Parallel()
	t.Run("transition_error_from_terminal", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowTransition(WfStatusCompleted, WfStatusRunning)
		var te *TransitionError
		require.ErrorAs(t,
			err, &te,
		)
		require.False(t,
			te.From !=
				RunStatus(WfStatusCompleted) || te.To != RunStatus(WfStatusRunning))
	})

	t.Run("unknown_status_error", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowTransition(WorkflowRunStatus("bogus"), WfStatusRunning)
		var ue *UnknownStatusError
		require.ErrorAs(t,
			err, &ue,
		)
		require.Equal(t,
			RunStatus("bogus"),

			ue.Status)
	})

	t.Run("self_transition_running", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowTransition(WfStatusRunning, WfStatusRunning)
		var te *TransitionError
		require.ErrorAs(t,
			err, &te,
		)
	})
}

func BenchmarkValidateWorkflowTransition(b *testing.B) {
	for b.Loop() {
		_ = ValidateWorkflowTransition(WfStatusRunning, WfStatusCompleted)
	}
}

func TestValidateStepTransition_AllValidTransitions(t *testing.T) {
	t.Parallel()
	for from, toStatuses := range validStepTransitions {
		for _, to := range toStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				assert.NoError(
					t, ValidateStepTransition(from, to))
			})
		}
	}
}

func TestValidateStepTransition_InvalidTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		from StepRunStatus
		to   StepRunStatus
	}{
		{"completed_to_running", StepCompleted, StepRunning},
		{"completed_to_pending", StepCompleted, StepPending},
		{"failed_to_running", StepFailed, StepRunning},
		{"failed_to_pending", StepFailed, StepPending},
		{"skipped_to_running", StepSkipped, StepRunning},
		{"skipped_to_pending", StepSkipped, StepPending},
		{"canceled_to_running", StepCanceled, StepRunning},
		{"canceled_to_pending", StepCanceled, StepPending},
		{"pending_to_failed", StepPending, StepFailed},
		{"waiting_to_failed", StepWaiting, StepFailed},
		{"running_to_running", StepRunning, StepRunning},
		{"running_to_pending", StepRunning, StepPending},
		{"running_to_waiting", StepRunning, StepWaiting},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateStepTransition(tc.from, tc.to)
			require.Error(t,
				err)
		})
	}
}

func TestValidateStepTransition_UnknownStatus(t *testing.T) {
	t.Parallel()
	err := ValidateStepTransition(StepRunStatus("unknown"), StepRunning)
	require.Error(t,
		err)

	var unknownErr *UnknownStatusError
	require.ErrorAs(t,
		err, &unknownErr)
}

func TestValidateStepTransition_TerminalHaveNoTransitions(t *testing.T) {
	t.Parallel()
	terminalStatuses := []StepRunStatus{StepCompleted, StepFailed, StepSkipped, StepCanceled}
	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			transitions, ok := validStepTransitions[status]
			assert.True(t,
				ok)
			assert.Empty(t, transitions)
		})
	}
}

func TestAllStepStatusesCovered(t *testing.T) {
	t.Parallel()
	allStatuses := []StepRunStatus{
		StepPending,
		StepWaiting,
		StepRunning,
		StepCompleted,
		StepFailed,
		StepSkipped,
		StepCanceled,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			assert.Contains(t, validStepTransitions, status)
		})
	}
	require.Len(t,
		validStepTransitions,

		len(allStatuses))
}

func TestValidateStepTransition_ErrorTypes(t *testing.T) {
	t.Parallel()
	t.Run("transition_error_from_terminal", func(t *testing.T) {
		t.Parallel()
		err := ValidateStepTransition(StepCompleted, StepRunning)
		var te *TransitionError
		require.ErrorAs(t,
			err, &te,
		)
		require.False(t,
			te.From !=
				RunStatus(StepCompleted) || te.To != RunStatus(StepRunning))
	})

	t.Run("unknown_status_error", func(t *testing.T) {
		t.Parallel()
		err := ValidateStepTransition(StepRunStatus("bogus"), StepRunning)
		var ue *UnknownStatusError
		require.ErrorAs(t,
			err, &ue,
		)
		require.Equal(t,
			RunStatus("bogus"),

			ue.Status)
	})

	t.Run("self_transition_running", func(t *testing.T) {
		t.Parallel()
		err := ValidateStepTransition(StepRunning, StepRunning)
		var te *TransitionError
		require.ErrorAs(t,
			err, &te,
		)
	})
}

func BenchmarkValidateStepTransition(b *testing.B) {
	for b.Loop() {
		_ = ValidateStepTransition(StepPending, StepRunning)
	}
}

func TestStepRunStatusIsTerminal_AllStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   StepRunStatus
		expected bool
	}{
		{StepPending, false},
		{StepWaiting, false},
		{StepRunning, false},
		{StepCompleted, true},
		{StepFailed, true},
		{StepSkipped, true},
		{StepCanceled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.status.IsTerminal())
		})
	}
}
