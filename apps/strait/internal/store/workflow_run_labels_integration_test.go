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
