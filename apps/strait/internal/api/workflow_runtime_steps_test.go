package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestServer_LoadWorkflowRunSteps(t *testing.T) {
	t.Parallel()

	t.Run("merges snapshot and dynamic runtime steps", func(t *testing.T) {
		t.Parallel()

		snapshotDefinition := domain.WorkflowSnapshotDefinition{
			Steps: []domain.WorkflowStep{{StepRef: "plan"}, {StepRef: "final", DependsOn: []string{"draft"}}},
		}
		snapshotJSON, err := json.Marshal(snapshotDefinition)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		storeMock := &APIStoreMock{
			GetWorkflowSnapshotFunc: func(_ context.Context, id string) (*domain.WorkflowSnapshot, error) {
				if id != "snap-1" {
					t.Fatalf("snapshot id = %q, want snap-1", id)
				}
				return &domain.WorkflowSnapshot{ID: id, Definition: snapshotJSON}, nil
			},
			ListDynamicWorkflowStepsByWorkflowRunFunc: func(_ context.Context, workflowRunID string) ([]domain.WorkflowStep, error) {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
				}
				return []domain.WorkflowStep{{StepRef: "draft", JobID: "job-draft", DependsOn: []string{"plan"}}}, nil
			},
		}

		server := &Server{store: storeMock}
		steps, err := server.loadWorkflowRunSteps(context.Background(), &domain.WorkflowRun{
			ID:                 "wr-1",
			WorkflowID:         "wf-1",
			WorkflowVersion:    1,
			WorkflowSnapshotID: "snap-1",
		})
		if err != nil {
			t.Fatalf("loadWorkflowRunSteps() error = %v", err)
		}
		if len(steps) != 3 {
			t.Fatalf("len(steps) = %d, want 3", len(steps))
		}
		if steps[2].StepRef != "draft" {
			t.Fatalf("steps[2].StepRef = %q, want draft", steps[2].StepRef)
		}
	})

	t.Run("rejects duplicate runtime step refs", func(t *testing.T) {
		t.Parallel()

		snapshotDefinition := domain.WorkflowSnapshotDefinition{
			Steps: []domain.WorkflowStep{{StepRef: "plan"}},
		}
		snapshotJSON, err := json.Marshal(snapshotDefinition)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		server := &Server{store: &APIStoreMock{
			GetWorkflowSnapshotFunc: func(_ context.Context, _ string) (*domain.WorkflowSnapshot, error) {
				return &domain.WorkflowSnapshot{ID: "snap-1", Definition: snapshotJSON}, nil
			},
			ListDynamicWorkflowStepsByWorkflowRunFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "plan"}}, nil
			},
		}}

		_, err = server.loadWorkflowRunSteps(context.Background(), &domain.WorkflowRun{
			ID:                 "wr-1",
			WorkflowID:         "wf-1",
			WorkflowVersion:    1,
			WorkflowSnapshotID: "snap-1",
		})
		if err == nil {
			t.Fatal("loadWorkflowRunSteps() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "duplicate runtime step_ref") {
			t.Fatalf("error = %q, want duplicate runtime step_ref", err)
		}
	})
}
