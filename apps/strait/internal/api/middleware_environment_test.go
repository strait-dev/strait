package api

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
)

// requireEnvironmentMatch unit tests.

func TestRequireEnvironmentMatch_NoCallerEnv(t *testing.T) {
	t.Parallel()
	if err := requireEnvironmentMatch(context.Background(), "env-prod"); err != nil {
		t.Fatalf("project-wide caller (no env in ctx) must pass through, got %v", err)
	}
	if err := requireEnvironmentMatch(context.Background(), ""); err != nil {
		t.Fatalf("project-wide caller against env-less resource must pass, got %v", err)
	}
}

func TestRequireEnvironmentMatch_Match(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxEnvironmentIDKey, "env-prod")
	if err := requireEnvironmentMatch(ctx, "env-prod"); err != nil {
		t.Fatalf("matching envs must pass, got %v", err)
	}
}

func TestRequireEnvironmentMatch_Mismatch(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxEnvironmentIDKey, "env-prod")
	if err := requireEnvironmentMatch(ctx, "env-staging"); !errors.Is(err, errEnvironmentMismatch) {
		t.Fatalf("mismatch must return errEnvironmentMismatch, got %v", err)
	}
}

func TestRequireEnvironmentMatch_EnvScopedKeyVsEnvlessResource(t *testing.T) {
	t.Parallel()
	// An env-bound key must NOT silently access env-less resources;
	// otherwise an env-prod key could reach legacy/unset jobs.
	ctx := context.WithValue(context.Background(), ctxEnvironmentIDKey, "env-prod")
	if err := requireEnvironmentMatch(ctx, ""); !errors.Is(err, errEnvironmentMismatch) {
		t.Fatalf("env-bound key against env-less resource must reject, got %v", err)
	}
}

// handler-level enforcement: trigger and read paths surface 404 when an
// environment-scoped key targets a job in a different environment.

func TestHandleGetJob_EnvironmentMismatch_Returns404(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", EnvironmentID: "env-staging", Enabled: true}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleGetJob(ctx, &GetJobInput{JobID: "job-1"})
	if err == nil {
		t.Fatal("expected error for env mismatch")
	}
	if !isNotFound(err) {
		t.Fatalf("expected 404 not found, got %v", err)
	}
}

func TestHandleGetJob_EnvironmentMatch_Allowed(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", EnvironmentID: "env-prod", Enabled: true}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	out, err := srv.handleGetJob(ctx, &GetJobInput{JobID: "job-1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out == nil || out.Body == nil || out.Body.ID != "job-1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestHandleGetRun_EnvironmentMismatch_Returns404(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleGetRun(ctx, &GetRunInput{RunID: "run-1"})
	if err == nil {
		t.Fatal("expected error for env mismatch via run.JobID")
	}
	if !isNotFound(err) {
		t.Fatalf("expected 404 not found, got %v", err)
	}
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	type statuser interface{ GetStatus() int }
	if s, ok := err.(statuser); ok && s.GetStatus() == 404 {
		return true
	}
	// huma.ErrorModel is wrapped; fall back to substring match on message.
	return errors.Is(err, errProjectMismatch) || errors.Is(err, errEnvironmentMismatch)
}
