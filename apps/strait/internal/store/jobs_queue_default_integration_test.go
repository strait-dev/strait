//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestIntegration_CreateJob_DefaultsEmptyQueueName(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID := "proj-default-queue"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{
		ID: projectID, OrgID: "org-1",
		Name: "default-queue",
	},
	),
	)

	job := &domain.Job{
		ProjectID:     projectID,
		Name:          "Worker Job",
		Slug:          "worker-job",
		ExecutionMode: domain.ExecutionModeWorker,
		TimeoutSecs:   60,
		MaxAttempts:   1,
	}
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "default",

		got.Queue,
	)

}
