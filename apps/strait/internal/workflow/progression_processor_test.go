package workflow

import (
	"context"
	"testing"

	"strait/internal/domain"
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

type fakeProgressionEventStore struct {
	events    []store.WorkflowProgressionEvent
	processed []int64
	released  []int64
}

func (s *fakeProgressionEventStore) ClaimWorkflowProgressionEvents(context.Context, int) ([]store.WorkflowProgressionEvent, error) {
	return s.events, nil
}

func (s *fakeProgressionEventStore) MarkWorkflowProgressionEventProcessed(_ context.Context, id int64) error {
	s.processed = append(s.processed, id)
	return nil
}

func (s *fakeProgressionEventStore) ReleaseWorkflowProgressionEvent(_ context.Context, id int64) error {
	s.released = append(s.released, id)
	return nil
}

func TestWorkflowProgression_ProcessOnceBatchesWorkflowContextLoad(t *testing.T) {
	ctx := context.Background()
	events := []store.WorkflowProgressionEvent{
		{ID: 1, WorkflowRunID: "wf-run", StepRunID: "step-run-a", StepRef: "a", Status: string(domain.StepCompleted)},
		{ID: 2, WorkflowRunID: "wf-run", StepRunID: "step-run-b", StepRef: "b", Status: string(domain.StepCompleted)},
	}
	eventStore := &fakeProgressionEventStore{events: events}
	stepRuns := map[string]*domain.WorkflowStepRun{
		"step-run-a": {ID: "step-run-a", WorkflowRunID: "wf-run", StepRef: "a", Status: domain.StepCompleted},
		"step-run-b": {ID: "step-run-b", WorkflowRunID: "wf-run", StepRef: "b", Status: domain.StepCompleted},
	}

	var listStepsCalls int
	callbackStore := &mockCallbackStore{
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			return stepRuns[id], nil
		},
		getWorkflowRunFn: func(context.Context, string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:              "wf-run",
				WorkflowID:      "wf",
				WorkflowVersion: 1,
				Status:          domain.WfStatusRunning,
			}, nil
		},
		listStepsByWorkflowVerFn: func(context.Context, string, int) ([]domain.WorkflowStep, error) {
			listStepsCalls++
			return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b"}}, nil
		},
		incrementStepDepsFn: func(context.Context, string, string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		listStepRunStatusesByWorkflowRunFn: func(context.Context, string) (map[string]domain.StepRunStatus, error) {
			return map[string]domain.StepRunStatus{"a": domain.StepCompleted, "b": domain.StepCompleted}, nil
		},
		listRunningStepRunsByWorkflowRunFn: func(context.Context, string, int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		listRunnableStepRunsByWorkflowRunFn: func(context.Context, string, int) ([]domain.WorkflowStepRun, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(context.Context, string) (int, error) {
			return 1, nil
		},
	}
	callback := NewStepCallback(callbackStore, &WorkflowEngine{}, nil)
	processor := NewProgressionProcessor(eventStore, callback, ProgressionProcessorConfig{Limit: 10})

	if err := processor.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if listStepsCalls != 1 {
		t.Fatalf("step definition loads = %d, want 1 for workflow batch", listStepsCalls)
	}
	if got := eventStore.processed; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("processed events = %v, want [1 2]", got)
	}
	if len(eventStore.released) != 0 {
		t.Fatalf("released events = %v, want none", eventStore.released)
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
	for range b.N {
		_ = groupProgressionEventsByWorkflow(events)
	}
}
