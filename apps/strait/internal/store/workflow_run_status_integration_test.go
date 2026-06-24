//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWorkflowRunStatus_RunNotFound_ReturnsErrWorkflowRunNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	nonExistentID := newID()
	err := q.UpdateWorkflowRunStatus(ctx, nonExistentID, domain.WfStatusPending, domain.WfStatusRunning, nil)
	assert.True(t, errors.Is(err, store.ErrWorkflowRunNotFound),
		"expected ErrWorkflowRunNotFound for missing run, got: %v", err)
}

func TestUpdateWorkflowRunStatus_WrongFromStatus_ReturnsConflictError(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-status-conflict-" + newID()
	wf := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "wf-run-status-conflict",
		Slug:      "wf-run-status-conflict-" + newID(),
		Enabled:   true,
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	run := &domain.WorkflowRun{
		ID:          newID(),
		WorkflowID:  wf.ID,
		ProjectID:   projectID,
		TriggeredBy: domain.TriggerManual,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx, run))
	// run is now in WfStatusPending

	// Attempt transition from WfStatusRunning (wrong) to WfStatusCompleted — run is actually pending.
	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	})
	require.Error(t, err, "expected an error when from status does not match actual status")
	assert.False(t, errors.Is(err, store.ErrWorkflowRunNotFound),
		"conflict error must not be ErrWorkflowRunNotFound, got: %v", err)
}

func TestUpdateWorkflowRunStatus_AlreadyInTargetStatus_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-status-idempotent-" + newID()
	wf := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "wf-run-status-idempotent",
		Slug:      "wf-run-status-idempotent-" + newID(),
		Enabled:   true,
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	run := &domain.WorkflowRun{
		ID:          newID(),
		WorkflowID:  wf.ID,
		ProjectID:   projectID,
		TriggeredBy: domain.TriggerManual,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx, run))
	// run is now in WfStatusPending

	// Transition pending -> running.
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": time.Now().UTC(),
	}))

	// Call again with the same from=pending, to=running; the run is already running,
	// so from does not match — but to matches the current state. The store should return nil (idempotent).
	err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": time.Now().UTC(),
	})
	assert.NoError(t, err, "expected nil when run is already in the target status (idempotent)")
}
