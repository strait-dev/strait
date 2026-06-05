//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRun.
		ID, labels))

	got, err := q.ListWorkflowRunLabels(ctx, wfRun.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "production",

		got["env"])
	require.Equal(t, "us-east-1",

		got["region"])

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
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRun.
		ID, map[string]string{"env": "staging"}))
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRun.
		ID, map[string]string{"env": "production"}))

	// Upsert with new value.

	got, err := q.ListWorkflowRunLabels(ctx, wfRun.ID)
	require.NoError(t, err)
	require.Equal(t, "production",

		got["env"])

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
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRun.
		ID, map[string]string{"env": "production"}))

	var xminBefore string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_run_labels
		WHERE workflow_run_id = $1 AND label_key = $2`,

		wfRun.ID, "env").Scan(&xminBefore))
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRun.
		ID, map[string]string{"env": "production"}))

	var xminAfterNoOp string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_run_labels
		WHERE workflow_run_id = $1 AND label_key = $2`,

		wfRun.ID, "env").Scan(&xminAfterNoOp))
	require.Equal(t, xminBefore,

		xminAfterNoOp,
	)
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRun.
		ID, map[string]string{"env": "staging"}))

	var xminAfterUpdate, labelValue string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, label_value
		FROM workflow_run_labels
		WHERE workflow_run_id = $1 AND label_key = $2`,

		wfRun.ID,
		"env").Scan(&xminAfterUpdate,
		&labelValue,
	))
	require.Equal(t, "staging",

		labelValue,
	)
	require.NotEqual(t, xminBefore,

		xminAfterUpdate,
	)

}

func TestCreateWorkflowRunLabels_EmptyLabels(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, newID(), map[string]string{}))

	// Should be a no-op.

}

func TestListWorkflowRunLabels_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.ListWorkflowRunLabels(ctx, newID())
	require.NoError(t, err)
	require.Len(t, got, 0)

}
