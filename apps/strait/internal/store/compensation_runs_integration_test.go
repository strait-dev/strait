//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestIntegration_CompensationRunsTrackTerminalJobResult(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-compensation-runs"
	if err := q.SetProjectContext(ctx, projectID); err != nil {
		t.Fatalf("SetProjectContext() error = %v", err)
	}
	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, projectID, domain.StepCompleted)
	compensationJob := mustCreateJob(t, ctx, q, projectID)
	jobRun := &domain.JobRun{
		ID:          newID(),
		JobID:       compensationJob.ID,
		ProjectID:   compensationJob.ProjectID,
		TriggeredBy: domain.TriggerWorkflow,
		Metadata: map[string]string{
			domain.RunMetadataCompensationWorkflowRunID: wfRun.ID,
			domain.RunMetadataCompensationStepRef:       stepRun.StepRef,
		},
	}
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun(compensation job) error = %v", err)
	}

	compRun := &domain.CompensationRun{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		StepRunID:         stepRun.ID,
		StepRef:           stepRun.StepRef,
		CompensationJobID: compensationJob.ID,
		JobRunID:          jobRun.ID,
		Status:            domain.CompensationPending,
		Input:             json.RawMessage(`{"undo":true}`),
	}
	if err := q.CreateCompensationRun(ctx, compRun); err != nil {
		t.Fatalf("CreateCompensationRun() error = %v", err)
	}

	startedAt := time.Now().Add(-time.Minute)
	if err := q.MarkCompensationRunStarted(ctx, compRun.ID, jobRun.ID, startedAt); err != nil {
		t.Fatalf("MarkCompensationRunStarted() error = %v", err)
	}

	got, err := q.MarkCompensationRunTerminalByJobRunID(ctx, jobRun.ID, domain.CompensationCompleted, json.RawMessage(`{"ok":true}`), "", time.Now())
	if err != nil {
		t.Fatalf("MarkCompensationRunTerminalByJobRunID() error = %v", err)
	}
	if got.Status != domain.CompensationCompleted || got.WorkflowRunID != wfRun.ID || got.JobRunID != jobRun.ID {
		t.Fatalf("terminal compensation run = %#v, want completed run linkage", got)
	}
	var output map[string]bool
	if err := json.Unmarshal(got.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !output["ok"] {
		t.Fatalf("output = %s, want persisted terminal output", string(got.Output))
	}

	remaining, err := q.CountIncompleteCompensationRuns(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("CountIncompleteCompensationRuns() error = %v", err)
	}
	if remaining != 0 {
		t.Fatalf("incomplete compensation runs = %d, want 0", remaining)
	}
}
