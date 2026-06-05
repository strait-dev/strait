//go:build integration

package store_test

import (
	"context"
	"fmt"
	"testing"

	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestIntegration_ListStepsByWorkflow_ReturnsMoreThanFiveHundredSteps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-large-workflow-" + newID()
	require.NoError(t, q.SetProjectContext(ctx,
		projectID,
	))

	name := "large workflow"
	slug := "large-workflow-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: &projectID,
		Name:      &name,
		Slug:      &slug,
	})
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})

	const stepCount = 501
	for i := 0; i < stepCount; i++ {
		stepRef := fmt.Sprintf("step-%03d", i)
		testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
			JobID:   &job.ID,
			StepRef: &stepRef,
		})
	}

	steps, err := q.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Len(t, steps, stepCount)

}
