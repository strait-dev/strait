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

func TestIntegration_GetRunTokenState_UsesSplitRunState(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const projectID = "proj-run-token-state-split"
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org-1", Name: "Run Token State Split"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	job := &domain.Job{
		ProjectID:     projectID,
		Name:          "Token Job Split",
		Slug:          "token-job-split",
		ExecutionMode: domain.ExecutionModeWorker,
		TimeoutSecs:   60,
		MaxAttempts:   3,
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	run := &domain.JobRun{
		ID:        "run-token-state-split",
		JobID:     job.ID,
		ProjectID: projectID,
		Status:    domain.StatusQueued,
		Attempt:   1,
		Payload:   []byte(`{"ok":true}`),
	}
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusExecuting, map[string]any{
		"attempt": 2,
	}); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
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

	if err := q.EnsureRunActiveForAttempt(ctx, run.ID, 2); err != nil {
		t.Fatalf("EnsureRunActiveForAttempt: %v", err)
	}
}
