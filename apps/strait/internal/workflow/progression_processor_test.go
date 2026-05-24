package workflow

import (
	"testing"

	"strait/internal/store"
)

func TestWorkflowProgression_GroupEventsByWorkflow(t *testing.T) {
	events := []store.WorkflowProgressionEvent{
		{ID: 1, WorkflowRunID: "wf-a"},
		{ID: 2, WorkflowRunID: "wf-b"},
		{ID: 3, WorkflowRunID: "wf-a"},
	}
	grouped := groupProgressionEventsByWorkflow(events)
	if len(grouped["wf-a"]) != 2 {
		t.Fatalf("wf-a group len = %d, want 2", len(grouped["wf-a"]))
	}
	if len(grouped["wf-b"]) != 1 {
		t.Fatalf("wf-b group len = %d, want 1", len(grouped["wf-b"]))
	}
}

func FuzzWorkflowProgression(f *testing.F) {
	f.Add("wf-a", "step-a", "completed")
	f.Add("", "", "")
	f.Fuzz(func(t *testing.T, workflowRunID, stepRunID, status string) {
		events := []store.WorkflowProgressionEvent{{
			WorkflowRunID: workflowRunID,
			StepRunID:     stepRunID,
			Status:        status,
		}}
		grouped := groupProgressionEventsByWorkflow(events)
		if len(grouped) != 1 {
			t.Fatalf("group count = %d, want 1", len(grouped))
		}
		if got := grouped[workflowRunID][0].StepRunID; got != stepRunID {
			t.Fatalf("stepRunID = %q, want %q", got, stepRunID)
		}
	})
}

func BenchmarkWorkflowProgression(b *testing.B) {
	events := make([]store.WorkflowProgressionEvent, 1000)
	for i := range events {
		events[i] = store.WorkflowProgressionEvent{WorkflowRunID: "wf"}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = groupProgressionEventsByWorkflow(events)
	}
}
