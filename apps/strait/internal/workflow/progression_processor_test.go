package workflow

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestWorkflowProgression_GroupEventsByWorkflow(t *testing.T) {
	events := []store.WorkflowProgressionEvent{
		{ID: 1, WorkflowRunID: "wf-a"},
		{ID: 2, WorkflowRunID: "wf-b"},
		{ID: 3, WorkflowRunID: "wf-a"},
	}
	grouped := groupProgressionEventsByWorkflow(events)
	require.Len(t, grouped["wf-a"],
		2)
	require.Len(t, grouped["wf-b"],
		1)

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

func (s *fakeProgressionEventStore) MarkWorkflowProgressionEventsProcessed(_ context.Context, ids []int64) error {
	s.processed = append(s.processed, ids...)
	return nil
}

func (s *fakeProgressionEventStore) ReleaseWorkflowProgressionEvent(_ context.Context, id int64) error {
	s.released = append(s.released, id)
	return nil
}

func (s *fakeProgressionEventStore) ReleaseWorkflowProgressionEvents(_ context.Context, ids []int64) error {
	s.released = append(s.released, ids...)
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
	var batchLoadCalls int
	var incrementBatchCalls int
	callbackStore := &mockCallbackStore{
		listWorkflowStepRunsByIDsFn: func(_ context.Context, ids []string) ([]domain.WorkflowStepRun, error) {
			batchLoadCalls++
			out := make([]domain.WorkflowStepRun, 0, len(ids))
			for _, id := range ids {
				out = append(out, *stepRuns[id])
			}
			return out, nil
		},
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
		incrementStepDepsBatchFn: func(context.Context, string, []string) ([]store.StepDepResult, error) {
			incrementBatchCalls++
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
		getWorkflowStepCompletionSummaryFn: func(context.Context, string) (store.WorkflowStepCompletionSummary, error) {
			return store.WorkflowStepCompletionSummary{NonTerminalCount: 1}, nil
		},
	}
	callback := NewStepCallback(callbackStore, &WorkflowEngine{}, nil)
	processor := NewProgressionProcessor(eventStore, callback, ProgressionProcessorConfig{Limit: 10})
	require.NoError(t,
		processor.
			ProcessOnce(ctx))
	require.EqualValues(t, 1,
		listStepsCalls,
	)
	require.EqualValues(t, 1,
		batchLoadCalls,
	)
	require.EqualValues(t, 1,
		incrementBatchCalls,
	)

	if got := eventStore.processed; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		require.Failf(t, "test failure",

			"processed events = %v, want [1 2]", got)
	}
	require.Len(t, eventStore.
		released,

		0)

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
		require.Len(t, grouped,
			1)
		require.Equal(t, stepRunID,
			grouped[workflowRunID][0].StepRunID,
		)

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
