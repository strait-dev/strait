package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var workflowStepKeySink string

func TestWorkflowStepKeys(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "approval id",
			got:  workflowApprovalID("step-run-123"),
			want: "approval:step-run-123",
		},
		{
			name: "approval event key",
			got:  workflowApprovalEventKey("workflow-run-456", "approve-invoice"),
			want: "approval:workflow-run-456:approve-invoice",
		},
		{
			name: "approval event trigger id",
			got:  workflowApprovalEventTriggerID("step-run-123"),
			want: "evt:approval:step-run-123",
		},
		{
			name: "cost gate approval id",
			got:  workflowCostGateApprovalID("step-run-123"),
			want: "costgate:step-run-123",
		},
		{
			name: "event trigger id",
			got:  workflowEventTriggerID("step-run-123"),
			want: "evt:step-run-123",
		},
		{
			name: "sleep trigger id",
			got:  workflowSleepTriggerID("step-run-123"),
			want: "slp:step-run-123",
		},
		{
			name: "sleep event key",
			got:  workflowSleepEventKey("workflow-run-456", "sleep-until-ready"),
			want: "sleep:workflow-run-456:sleep-until-ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func BenchmarkWorkflowStepKeys(b *testing.B) {
	stepRunID := "step-run-01JYQ7S5H2W8M9N3Q4R5T6V7WX"
	workflowRunID := "workflow-run-01JYQ7S5H2W8M9N3Q4R5T6V7WY"
	stepRef := "approval-gate-for-regional-fulfillment"

	b.Run("ApprovalID", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowApprovalID(stepRunID)
		}
	})
	b.Run("ApprovalEventKey", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowApprovalEventKey(workflowRunID, stepRef)
		}
	})
	b.Run("ApprovalEventTriggerID", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowApprovalEventTriggerID(stepRunID)
		}
	})
	b.Run("CostGateApprovalID", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowCostGateApprovalID(stepRunID)
		}
	})
	b.Run("EventTriggerID", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowEventTriggerID(stepRunID)
		}
	})
	b.Run("SleepTriggerID", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowSleepTriggerID(stepRunID)
		}
	})
	b.Run("SleepEventKey", func(b *testing.B) {
		for b.Loop() {
			workflowStepKeySink = workflowSleepEventKey(workflowRunID, stepRef)
		}
	})
}
