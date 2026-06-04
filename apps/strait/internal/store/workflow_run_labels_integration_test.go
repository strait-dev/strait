//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/testutil"
)

func TestCreateWorkflowRunLabels(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-labels-create"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	labels := map[string]string{
		"env":    "production",
		"region": "us-east-1",
	}
	if err := q.CreateWorkflowRunLabels(ctx, wfRun.ID, labels); err != nil {
		t.Fatalf("CreateWorkflowRunLabels() error = %v", err)
	}

	got, err := q.ListWorkflowRunLabels(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListWorkflowRunLabels() len = %d, want 2", len(got))
	}
	if got["env"] != "production" {
		t.Fatalf("label[env] = %q, want %q", got["env"], "production")
	}
	if got["region"] != "us-east-1" {
		t.Fatalf("label[region] = %q, want %q", got["region"], "us-east-1")
	}
}

func TestCreateWorkflowRunLabels_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-labels-upsert"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	if err := q.CreateWorkflowRunLabels(ctx, wfRun.ID, map[string]string{"env": "staging"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels() error = %v", err)
	}

	// Upsert with new value.
	if err := q.CreateWorkflowRunLabels(ctx, wfRun.ID, map[string]string{"env": "production"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(upsert) error = %v", err)
	}

	got, err := q.ListWorkflowRunLabels(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels() error = %v", err)
	}
	if got["env"] != "production" {
		t.Fatalf("label[env] = %q, want %q", got["env"], "production")
	}
}

func TestCreateWorkflowRunLabels_SameValueNoOpDoesNotRewrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-labels-no-op"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})

	if err := q.CreateWorkflowRunLabels(ctx, wfRun.ID, map[string]string{"env": "production"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels() error = %v", err)
	}

	var xminBefore string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_run_labels
		WHERE workflow_run_id = $1 AND label_key = $2`,
		wfRun.ID, "env",
	).Scan(&xminBefore); err != nil {
		t.Fatalf("query workflow run label xmin before no-op: %v", err)
	}

	if err := q.CreateWorkflowRunLabels(ctx, wfRun.ID, map[string]string{"env": "production"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(no-op) error = %v", err)
	}

	var xminAfterNoOp string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_run_labels
		WHERE workflow_run_id = $1 AND label_key = $2`,
		wfRun.ID, "env",
	).Scan(&xminAfterNoOp); err != nil {
		t.Fatalf("query workflow run label xmin after no-op: %v", err)
	}
	if xminAfterNoOp != xminBefore {
		t.Fatalf("same-value upsert changed xmin from %s to %s", xminBefore, xminAfterNoOp)
	}

	if err := q.CreateWorkflowRunLabels(ctx, wfRun.ID, map[string]string{"env": "staging"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(update) error = %v", err)
	}

	var xminAfterUpdate, labelValue string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, label_value
		FROM workflow_run_labels
		WHERE workflow_run_id = $1 AND label_key = $2`,
		wfRun.ID, "env",
	).Scan(&xminAfterUpdate, &labelValue); err != nil {
		t.Fatalf("query workflow run label after update: %v", err)
	}
	if labelValue != "staging" {
		t.Fatalf("label value = %q, want %q", labelValue, "staging")
	}
	if xminAfterUpdate == xminBefore {
		t.Fatalf("changed-value upsert preserved xmin %s", xminAfterUpdate)
	}
}

func TestCreateWorkflowRunLabels_EmptyLabels(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Should be a no-op.
	if err := q.CreateWorkflowRunLabels(ctx, newID(), map[string]string{}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(empty) error = %v", err)
	}
}

func TestListWorkflowRunLabels_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.ListWorkflowRunLabels(ctx, newID())
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels(empty) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListWorkflowRunLabels(empty) len = %d, want 0", len(got))
	}
}
