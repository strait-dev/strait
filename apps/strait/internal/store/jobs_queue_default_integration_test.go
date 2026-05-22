//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestIntegration_CreateJob_DefaultsEmptyQueueName(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID := "proj-default-queue"
	if err := q.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-1",
		Name:  "default-queue",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	job := &domain.Job{
		ProjectID:     projectID,
		Name:          "Worker Job",
		Slug:          "worker-job",
		ExecutionMode: domain.ExecutionModeWorker,
		TimeoutSecs:   60,
		MaxAttempts:   1,
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Queue != "default" {
		t.Fatalf("got.Queue = %q, want default", got.Queue)
	}
}
