package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

func TestLoadRunCreationJobRejectsBlankID(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			t.Fatal("GetJob must not run for blank job ID")
			return nil, nil
		},
	}}

	_, err := srv.loadRunCreationJob(context.Background(), " ", "test.project_match", "testHandler")
	assertStatusError(t, err, http.StatusBadRequest, "job_id is required")
}

func TestLoadRunCreationJobMapsMissingJobToNotFound(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			if jobID != "job-missing" {
				t.Fatalf("jobID = %q, want job-missing", jobID)
			}
			return nil, store.ErrJobNotFound
		},
	}}

	_, err := srv.loadRunCreationJob(context.Background(), "job-missing", "test.project_match", "testHandler")
	assertStatusError(t, err, http.StatusNotFound, "job not found")
}

func TestLoadRunCreationJobHidesProjectMismatch(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			return &domain.Job{ID: jobID, ProjectID: "project-owner"}, nil
		},
	}}
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "project-caller")

	_, err := srv.loadRunCreationJob(ctx, "job-1", "test.project_match", "testHandler")
	assertStatusError(t, err, http.StatusNotFound, "job not found")
}

func TestLoadRunCreationJobReturnsScopedJob(t *testing.T) {
	t.Parallel()

	want := &domain.Job{ID: "job-1", ProjectID: "project-1", EnvironmentID: "env-1"}
	srv := newTestServer(t, &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			if jobID != want.ID {
				t.Fatalf("jobID = %q, want %q", jobID, want.ID)
			}
			return want, nil
		},
	}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, want.ProjectID)
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, want.EnvironmentID)

	got, err := srv.loadRunCreationJob(ctx, want.ID, "test.project_match", "testHandler")
	if err != nil {
		t.Fatalf("loadRunCreationJob() error = %v", err)
	}
	if got != want {
		t.Fatalf("loadRunCreationJob() = %p, want %p", got, want)
	}
}

func assertStatusError(t *testing.T, err error, status int, contains string) {
	t.Helper()

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T, want huma.StatusError", err)
	}
	if statusErr.GetStatus() != status {
		t.Fatalf("status = %d, want %d", statusErr.GetStatus(), status)
	}
	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Fatalf("error = %q, want %q", err.Error(), contains)
	}
}
