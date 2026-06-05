//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestIntegration_CompensationRunsTrackTerminalJobResult(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-compensation-runs"
	require.NoError(t, q.SetProjectContext(ctx,
		projectID))

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
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

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
	require.NoError(t, q.CreateCompensationRun(ctx,
		compRun))

	startedAt := time.Now().Add(-time.Minute)
	require.NoError(t, q.MarkCompensationRunStarted(ctx, compRun.
		ID, jobRun.
		ID, startedAt,
	))

	got, err := q.MarkCompensationRunTerminalByJobRunID(ctx, jobRun.ID, domain.CompensationCompleted, json.RawMessage(`{"ok":true}`), "", time.Now())
	require.NoError(t, err)
	require.False(t, got.Status !=
		domain.
			CompensationCompleted ||
		got.WorkflowRunID !=
			wfRun.ID || got.JobRunID !=
		jobRun.ID)

	var output map[string]bool
	require.NoError(t, json.
		Unmarshal(got.Output,
			&output))
	require.True(t, output["ok"])

	remaining, err := q.CountIncompleteCompensationRuns(ctx, wfRun.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, remaining)

}
