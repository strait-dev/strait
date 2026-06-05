//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestDeleteWorkflow_DeletesTerminalRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-workflow-" + newID()
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "delete-workflow", Slug: "delete-workflow-" + newID(), Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusCompleted, TriggeredBy: domain.TriggerManual}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		trigger))
	require.NoError(t, q.DeleteWorkflow(ctx, wf.
		ID))

	if _, err := q.GetWorkflow(ctx, wf.ID); !errors.Is(err, store.ErrWorkflowNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflow() error = %v, want ErrWorkflowNotFound", err)
	}
	if _, err := q.GetWorkflowRun(ctx, run.ID); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflowRun() error = %v, want ErrWorkflowRunNotFound", err)
	}
	gotTrigger, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.Nil(t, gotTrigger)

}

func TestDeleteWorkflow_RejectsActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-active-workflow-" + newID()
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "delete-active-workflow", Slug: "delete-active-workflow-" + newID(), Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: domain.TriggerManual}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

	if err := q.DeleteWorkflow(ctx, wf.ID); !errors.Is(err, store.ErrWorkflowHasActiveRuns) {
		require.Failf(t, "test failure",

			"DeleteWorkflow() error = %v, want ErrWorkflowHasActiveRuns", err)
	}
	if _, err := q.GetWorkflow(ctx, wf.ID); err != nil {
		require.Failf(t, "test failure",

			"GetWorkflow() after rejected delete error = %v", err)
	}
}
