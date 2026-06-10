package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

func TestLoadRunCreationJobRejectsBlankID(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			require.Fail(t,

				"GetJob must not run for blank job ID")
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
			require.Equal(t, "job-missing",
				jobID,
			)

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
			require.Equal(t, want.ID,
				jobID)

			return want, nil
		},
	}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, want.ProjectID)
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, want.EnvironmentID)

	got, err := srv.loadRunCreationJob(ctx, want.ID, "test.project_match", "testHandler")
	require.NoError(t, err)
	require.Equal(t, want,
		got)
}

func assertStatusError(t *testing.T, err error, status int, contains string) {
	t.Helper()

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, status,
		statusErr.
			GetStatus(),
	)
	require.False(t, contains !=
		"" &&
		!strings.Contains(err.
			Error(), contains))
}
