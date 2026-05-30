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

func TestDeleteWorkflowRunsFinishedBefore_IncludesAllTerminalStatuses(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-retention-terminal-" + newID()
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-retention-terminal", Slug: "wf-retention-terminal-" + newID(), Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	oldFinishedAt := time.Now().UTC().Add(-72 * time.Hour)
	terminalStatuses := []domain.WorkflowRunStatus{
		domain.WfStatusTimedOut,
		domain.WfStatusCompensated,
		domain.WfStatusCompensationFailed,
		domain.WfStatusContinued,
	}
	deletedIDs := make([]string, 0, len(terminalStatuses))
	for _, status := range terminalStatuses {
		run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: status, TriggeredBy: domain.TriggerManual}
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			t.Fatalf("CreateWorkflowRun(%s) error = %v", status, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1 WHERE id = $2`, oldFinishedAt, run.ID); err != nil {
			t.Fatalf("set finished_at for %s: %v", status, err)
		}
		deletedIDs = append(deletedIDs, run.ID)
	}

	active := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusCompensating, TriggeredBy: domain.TriggerManual}
	if err := q.CreateWorkflowRun(ctx, active); err != nil {
		t.Fatalf("CreateWorkflowRun(compensating) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1 WHERE id = $2`, oldFinishedAt, active.ID); err != nil {
		t.Fatalf("set finished_at for compensating: %v", err)
	}

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsFinishedBefore() error = %v", err)
	}
	if deleted != int64(len(deletedIDs)) {
		t.Fatalf("deleted = %d, want %d", deleted, len(deletedIDs))
	}

	for _, id := range deletedIDs {
		_, err := q.GetWorkflowRun(ctx, id)
		if !errors.Is(err, store.ErrWorkflowRunNotFound) {
			t.Fatalf("GetWorkflowRun(%s) error = %v, want ErrWorkflowRunNotFound", id, err)
		}
	}
	if _, err := q.GetWorkflowRun(ctx, active.ID); err != nil {
		t.Fatalf("GetWorkflowRun(active compensating) error = %v", err)
	}
}

func TestDeleteWorkflowRunsByOrgOlderThan_IncludesAllTerminalStatuses(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-wf-retention-terminal-" + newID()
	projectID := "proj-wf-retention-terminal-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-org-retention", Slug: "wf-org-retention-" + newID(), Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	oldFinishedAt := time.Now().UTC().Add(-72 * time.Hour)
	terminalStatuses := []domain.WorkflowRunStatus{
		domain.WfStatusTimedOut,
		domain.WfStatusCompensated,
		domain.WfStatusCompensationFailed,
		domain.WfStatusContinued,
	}
	deletedIDs := make([]string, 0, len(terminalStatuses))
	for _, status := range terminalStatuses {
		run := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: status, TriggeredBy: domain.TriggerManual}
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			t.Fatalf("CreateWorkflowRun(%s) error = %v", status, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1 WHERE id = $2`, oldFinishedAt, run.ID); err != nil {
			t.Fatalf("set finished_at for %s: %v", status, err)
		}
		deletedIDs = append(deletedIDs, run.ID)
	}

	deleted, err := q.DeleteWorkflowRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsByOrgOlderThan() error = %v", err)
	}
	if deleted != int64(len(deletedIDs)) {
		t.Fatalf("deleted = %d, want %d", deleted, len(deletedIDs))
	}

	for _, id := range deletedIDs {
		_, err := q.GetWorkflowRun(ctx, id)
		if !errors.Is(err, store.ErrWorkflowRunNotFound) {
			t.Fatalf("GetWorkflowRun(%s) error = %v, want ErrWorkflowRunNotFound", id, err)
		}
	}
}
