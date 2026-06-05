//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestIntegration_GetRunTokenState_ReturnsStatusAttemptAndProject(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const projectID = "proj-run-token-state"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{
		ID: projectID, OrgID: "org-1",
		Name: "Run Token State",
	}))

	job := &domain.Job{
		ProjectID:     projectID,
		Name:          "Token Job",
		Slug:          "token-job",
		ExecutionMode: domain.ExecutionModeWorker,
		TimeoutSecs:   60,
		MaxAttempts:   3,
	}
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := &domain.JobRun{
		ID:        "run-token-state",
		JobID:     job.ID,
		ProjectID: projectID,
		Status:    domain.StatusExecuting,
		Attempt:   2,
		Payload:   []byte(`{"ok":true}`),
	}
	require.NoError(t, q.CreateRun(ctx,
		run))

	status, attempt, gotProjectID, err := q.GetRunTokenState(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		status,
	)
	require.EqualValues(t, 2, attempt)
	require.Equal(t, projectID,

		gotProjectID,
	)

}

func TestIntegration_GetRunTokenState_UsesSplitRunState(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const projectID = "proj-run-token-state-split"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{
		ID: projectID, OrgID: "org-1",
		Name: "Run Token State Split",
	}))

	job := &domain.Job{
		ProjectID:     projectID,
		Name:          "Token Job Split",
		Slug:          "token-job-split",
		ExecutionMode: domain.ExecutionModeWorker,
		TimeoutSecs:   60,
		MaxAttempts:   3,
	}
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := &domain.JobRun{
		ID:        "run-token-state-split",
		JobID:     job.ID,
		ProjectID: projectID,
		Status:    domain.StatusQueued,
		Attempt:   1,
		Payload:   []byte(`{"ok":true}`),
	}
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusQueued, domain.StatusExecuting,
		map[string]any{
			"attempt": 2}))

	status, attempt, gotProjectID, err := q.GetRunTokenState(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusExecuting,
		status,
	)
	require.EqualValues(t, 2, attempt)
	require.Equal(t, projectID,

		gotProjectID,
	)
	require.NoError(t, q.EnsureRunActiveForAttempt(ctx, run.
		ID, 2))

}
