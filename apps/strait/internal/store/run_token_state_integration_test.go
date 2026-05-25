//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestIntegration_GetRunTokenState_ReturnsStatusAttemptAndProject(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const projectID = "proj-run-token-state"
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-1", Name: "Run Token State"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := &domain.Job{
		ProjectID:     projectID,
		Name:          "Token Job",
		Slug:          "token-job",
		ExecutionMode: domain.ExecutionModeWorker,
		TimeoutSecs:   60,
		MaxAttempts:   3,
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	run := &domain.JobRun{
		ID:        "run-token-state",
		JobID:     job.ID,
		ProjectID: projectID,
		Status:    domain.StatusExecuting,
		Attempt:   2,
		Payload:   []byte(`{"ok":true}`),
	}
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	status, attempt, gotProjectID, err := q.GetRunTokenState(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunTokenState: %v", err)
	}
	if status != domain.StatusExecuting {
		t.Fatalf("status = %q, want %q", status, domain.StatusExecuting)
	}
	if attempt != 2 {
		t.Fatalf("attempt = %d, want 2", attempt)
	}
	if gotProjectID != projectID {
		t.Fatalf("projectID = %q, want %q", gotProjectID, projectID)
	}
}
