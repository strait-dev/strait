package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestTenantIso_JobDeps_List_RejectsCrossProject verifies that a caller
// in project A cannot list dependencies on a job that belongs to project B.
func TestTenantIso_JobDeps_List_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-bbb"}, nil
		},
		ListJobDependenciesFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobDependency, error) {
			t.Fatal("ListJobDependencies must not be called for cross-project access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleListJobDependencies(ctx, &ListJobDependenciesInput{JobID: "job-foreign"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

// TestTenantIso_JobDeps_List_RejectsCrossEnv ensures env scoping is honored
// on the dependency listing path.
func TestTenantIso_JobDeps_List_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-aaa", EnvironmentID: "env-staging"}, nil
		},
		ListJobDependenciesFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobDependency, error) {
			t.Fatal("ListJobDependencies must not be called for cross-env access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleListJobDependencies(ctx, &ListJobDependenciesInput{JobID: "job-1"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

// TestTenantIso_JobDeps_Delete_RejectsCrossProject ensures that the delete
// path does not silently succeed when called against a foreign job; the
// dependency must remain intact.
func TestTenantIso_JobDeps_Delete_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-bbb"}, nil
		},
		GetJobDependencyFunc: func(_ context.Context, id string) (*domain.JobDependency, error) {
			return &domain.JobDependency{ID: id, JobID: "job-foreign"}, nil
		},
		DeleteJobDependencyFunc: func(_ context.Context, _ string) error {
			t.Fatal("DeleteJobDependency must not be called for cross-project access")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleDeleteJobDependency(ctx, &DeleteJobDependencyInput{JobID: "job-foreign", DepID: "dep-1"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}
