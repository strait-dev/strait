package domain

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkflowRunStatus_IsTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   WorkflowRunStatus
		expected bool
	}{
		{WfStatusPending, false},
		{WfStatusRunning, false},
		{WfStatusCompleted, true},
		{WfStatusFailed, true},
		{WfStatusCanceled, true},
	}

	got := make(map[WorkflowRunStatus]bool, len(tests))
	want := make(map[WorkflowRunStatus]bool, len(tests))
	for _, tc := range tests {
		got[tc.status] = tc.status.IsTerminal()
		want[tc.status] = tc.expected
	}
	require.True(t,
		reflect.DeepEqual(got, want))

}

func TestStepRunStatus_IsTerminal(t *testing.T) {
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

	got := make(map[StepRunStatus]bool, len(tests))
	want := make(map[StepRunStatus]bool, len(tests))
	for _, tc := range tests {
		got[tc.status] = tc.status.IsTerminal()
		want[tc.status] = tc.expected
	}
	require.True(t,
		reflect.DeepEqual(got, want))

}

func TestEventTriggerStatusConstants(t *testing.T) {
	t.Parallel()
	require.False(t,
		EventTriggerStatusWaiting !=
			"waiting" || EventTriggerStatusReceived !=
			"received" ||
			EventTriggerStatusTimedOut !=
				"timed_out" ||
			EventTriggerStatusCanceled !=
				"canceled")

}

func TestWorkflowStepTypeWaitForEvent(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		"wait_for_event",

		string(WorkflowStepTypeWaitForEvent))

	// Verify it is distinct from existing step types.
	types := []WorkflowStepType{
		WorkflowStepTypeJob,
		WorkflowStepTypeApproval,
		WorkflowStepTypeSubWorkflow,
		WorkflowStepTypeWaitForEvent,
	}
	seen := make(map[WorkflowStepType]struct{}, len(types))
	for _, st := range types {
		require.NotContains(t, seen, st)
		seen[st] = struct{}{}
	}
}

func TestDefaultEventTimeoutSecs(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		3600, DefaultEventTimeoutSecs,
	)

}

func TestDeploymentStrategy_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		strategy DeploymentStrategy
		valid    bool
	}{
		{DeploymentStrategyDirect, true},
		{DeploymentStrategyCanary, true},
	}
	for _, tc := range tests {
		require.Equal(t, tc.valid, tc.strategy.IsValid())
	}
}

func TestDeploymentStrategy_Invalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		strategy DeploymentStrategy
	}{
		{DeploymentStrategy("blue-green")},
		{DeploymentStrategy("")},
		{DeploymentStrategy("rolling")},
	}
	for _, tc := range tests {
		require.False(t,
			tc.strategy.
				IsValid())

	}
}
