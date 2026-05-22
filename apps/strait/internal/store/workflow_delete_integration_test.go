//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestDeleteWorkflow_DeletesTerminalRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-workflow-" + newID()
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "delete-workflow", Slug: "delete-workflow-" + newID(), Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusCompleted, TriggeredBy: domain.TriggerManual}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	trigger := &domain.EventTrigger{
		ID:            newID(),
		EventKey:      "event-delete-workflow-" + newID(),
		ProjectID:     projectID,
		SourceType:    "workflow_step",
		WorkflowRunID: run.ID,
		Status:        "received",
		TimeoutSecs:   300,
		RequestedAt:   time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(5 * time.Minute),
	}
	if err := q.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	if err := q.DeleteWorkflow(ctx, wf.ID); err != nil {
		t.Fatalf("DeleteWorkflow() error = %v", err)
	}
	if _, err := q.GetWorkflow(ctx, wf.ID); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflow() error = %v, want ErrWorkflowNotFound", err)
	}
	if _, err := q.GetWorkflowRun(ctx, run.ID); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Fatalf("GetWorkflowRun() error = %v, want ErrWorkflowRunNotFound", err)
	}
	gotTrigger, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if gotTrigger != nil {
		t.Fatalf("GetEventTriggerByEventKey() = %+v, want nil", gotTrigger)
	}
}

func TestDeleteWorkflow_RejectsActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-active-workflow-" + newID()
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "delete-active-workflow", Slug: "delete-active-workflow-" + newID(), Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: domain.TriggerManual}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	if err := q.DeleteWorkflow(ctx, wf.ID); !errors.Is(err, store.ErrWorkflowHasActiveRuns) {
		t.Fatalf("DeleteWorkflow() error = %v, want ErrWorkflowHasActiveRuns", err)
	}
	if _, err := q.GetWorkflow(ctx, wf.ID); err != nil {
		t.Fatalf("GetWorkflow() after rejected delete error = %v", err)
	}
}
