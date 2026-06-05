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

func TestDeleteWorkflowRunsFinishedBefore_IncludesAllTerminalStatuses(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-retention-terminal-" + newID()
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-retention-terminal", Slug: "wf-retention-terminal-" + newID(), Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	oldFinishedAt := time.Now().UTC().Add(-72 * time.Hour)
	terminalStatuses := []domain.WorkflowRunStatus{
		domain.WfStatusTimedOut,
		domain.WfStatusCompensated,
		domain.WfStatusCompensationFailed,
	}
	deletedIDs := make([]string, 0, len(terminalStatuses))
	for _, status := range terminalStatuses {
		run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: status, TriggeredBy: domain.TriggerManual}
		require.NoError(t, q.CreateWorkflowRun(ctx,
			run))

		if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1 WHERE id = $2`, oldFinishedAt, run.ID); err != nil {
			require.Failf(t, "test failure",

				"set finished_at for %s: %v", status, err)
		}
		deletedIDs = append(deletedIDs, run.ID)
	}

	active := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusCompensating, TriggeredBy: domain.TriggerManual}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		active))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1 WHERE id = $2`, oldFinishedAt, active.ID); err != nil {
		require.Failf(t, "test failure",

			"set finished_at for compensating: %v", err)
	}

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	require.NoError(t, err)
	require.Equal(t, int64(
		len(deletedIDs)), deleted,
	)

	for _, id := range deletedIDs {
		_, err := q.GetWorkflowRun(ctx, id)
		require.True(t, errors.Is(err, store.
			ErrWorkflowRunNotFound,
		))

	}
	if _, err := q.GetWorkflowRun(ctx, active.ID); err != nil {
		require.Failf(t, "test failure",

			"GetWorkflowRun(active compensating) error = %v", err)
	}
}

func TestDeleteWorkflowRunsByOrgOlderThan_IncludesAllTerminalStatuses(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-wf-retention-terminal-" + newID()
	projectID := "proj-wf-retention-terminal-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,
		OrgID: orgID, Name: "P"}))

	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-org-retention", Slug: "wf-org-retention-" + newID(), Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	oldFinishedAt := time.Now().UTC().Add(-72 * time.Hour)
	terminalStatuses := []domain.WorkflowRunStatus{
		domain.WfStatusTimedOut,
		domain.WfStatusCompensated,
		domain.WfStatusCompensationFailed,
	}
	deletedIDs := make([]string, 0, len(terminalStatuses))
	for _, status := range terminalStatuses {
		run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: status, TriggeredBy: domain.TriggerManual}
		require.NoError(t, q.CreateWorkflowRun(ctx,
			run))

		if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1 WHERE id = $2`, oldFinishedAt, run.ID); err != nil {
			require.Failf(t, "test failure",

				"set finished_at for %s: %v", status, err)
		}
		deletedIDs = append(deletedIDs, run.ID)
	}

	deleted, err := q.DeleteWorkflowRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(
		len(deletedIDs)), deleted,
	)

	for _, id := range deletedIDs {
		_, err := q.GetWorkflowRun(ctx, id)
		require.True(t, errors.Is(err, store.
			ErrWorkflowRunNotFound,
		))

	}
}
