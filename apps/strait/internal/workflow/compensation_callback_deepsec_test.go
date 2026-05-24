package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestOnJobRunTerminal_CompensationCompletionMarksWorkflowCompensated(t *testing.T) {
	t.Parallel()

	var terminalStatus string
	var workflowTo domain.WorkflowRunStatus
	store := &mockCallbackStore{
		markCompensationRunTerminalFn: func(_ context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error) {
			if jobRunID != "jr-comp" {
				t.Fatalf("jobRunID = %q, want jr-comp", jobRunID)
			}
			if status != domain.CompensationCompleted {
				t.Fatalf("compensation status = %q, want completed", status)
			}
			if string(output) != `{"refunded":true}` {
				t.Fatalf("output = %s, want refund result", string(output))
			}
			if errMsg != "" {
				t.Fatalf("errMsg = %q, want empty", errMsg)
			}
			terminalStatus = status
			return &domain.CompensationRun{ID: "cr-1", WorkflowRunID: "wfr-1", Status: status}, nil
		},
		countIncompleteCompensationRunsFn: func(_ context.Context, workflowRunID string) (int, error) {
			if workflowRunID != "wfr-1" {
				t.Fatalf("workflowRunID = %q, want wfr-1", workflowRunID)
			}
			return 0, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			if id != "wfr-1" || from != domain.WfStatusCompensating || to != domain.WfStatusCompensated {
				t.Fatalf("UpdateWorkflowRunStatus(%q, %q, %q), want compensating -> compensated", id, from, to)
			}
			if fields["finished_at"] == nil {
				t.Fatalf("finished_at not set: %#v", fields)
			}
			workflowTo = to
			return nil
		},
	}
	cb := NewStepCallback(store, nil, nil)

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:     "jr-comp",
		Status: domain.StatusCompleted,
		Result: json.RawMessage(`{"refunded":true}`),
		Metadata: map[string]string{
			domain.RunMetadataCompensationRunID:         "cr-1",
			domain.RunMetadataCompensationWorkflowRunID: "wfr-1",
			domain.RunMetadataCompensationStepRef:       "charge-card",
		},
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}
	if terminalStatus != domain.CompensationCompleted || workflowTo != domain.WfStatusCompensated {
		t.Fatalf("terminalStatus=%q workflowTo=%q, want completed/compensated", terminalStatus, workflowTo)
	}
}

func TestOnJobRunTerminal_CompensationFailureMarksWorkflowFailed(t *testing.T) {
	t.Parallel()

	var workflowTo domain.WorkflowRunStatus
	store := &mockCallbackStore{
		markCompensationRunTerminalFn: func(_ context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error) {
			if status != domain.CompensationFailed {
				t.Fatalf("compensation status = %q, want failed", status)
			}
			if errMsg != "refund failed" {
				t.Fatalf("errMsg = %q, want refund failed", errMsg)
			}
			return &domain.CompensationRun{ID: "cr-1", WorkflowRunID: "wfr-1", Status: status}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			if id != "wfr-1" || from != domain.WfStatusCompensating || to != domain.WfStatusCompensationFailed {
				t.Fatalf("UpdateWorkflowRunStatus(%q, %q, %q), want compensating -> compensation_failed", id, from, to)
			}
			if fields["error"] != "refund failed" {
				t.Fatalf("error field = %#v, want refund failed", fields["error"])
			}
			workflowTo = to
			return nil
		},
	}
	cb := NewStepCallback(store, nil, nil)

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:     "jr-comp",
		Status: domain.StatusFailed,
		Error:  "refund failed",
		Metadata: map[string]string{
			domain.RunMetadataCompensationRunID:         "cr-1",
			domain.RunMetadataCompensationWorkflowRunID: "wfr-1",
			domain.RunMetadataCompensationStepRef:       "charge-card",
		},
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}
	if workflowTo != domain.WfStatusCompensationFailed {
		t.Fatalf("workflowTo = %q, want compensation_failed", workflowTo)
	}
}
