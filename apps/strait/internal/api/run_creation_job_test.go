package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestLoadRunCreationJobUsesTriggerJobCache(t *testing.T) {
	job := &domain.Job{
		ID:                "job-1",
		ProjectID:         "project-1",
		EnvironmentID:     "env-1",
		Enabled:           true,
		Tags:              map[string]string{"stable": "yes"},
		PayloadSchema:     []byte(`{"type":"object"}`),
		CacheVersion:      1,
		ExecutionMode:     domain.ExecutionModeHTTP,
		EndpointURL:       "https://example.com/job",
		MaxAttempts:       1,
		TimeoutSecs:       30,
		Version:           1,
		VersionID:         "ver_1",
		VersionPolicy:     domain.VersionPolicyPin,
		CronOverlapPolicy: domain.OverlapPolicyAllow,
	}
	calls := 0
	srv := &Server{
		store: &APIStoreMock{
			GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
				calls++
				require.Equal(t, job.ID, jobID)
				return job, nil
			},
		},
		triggerJobCache: newTriggerJobCache(time.Minute),
	}
	t.Cleanup(srv.triggerJobCache.Stop)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, job.ProjectID)
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, job.EnvironmentID)

	got, err := srv.loadRunCreationJob(ctx, job.ID, "test.project_match", "testHandler")
	require.NoError(t, err)
	require.Equal(t, "yes", got.Tags["stable"])

	got.Tags["stable"] = "mutated"
	got.PayloadSchema[0] = '['

	got, err = srv.loadRunCreationJob(ctx, job.ID, "test.project_match", "testHandler")
	require.NoError(t, err)
	require.Equal(t, "yes", got.Tags["stable"])
	require.Equal(t, []byte(`{"type":"object"}`), []byte(got.PayloadSchema))
	require.Equal(t, 1, calls)
}

func TestLoadRunCreationJobReloadsAfterTriggerJobCacheInvalidation(t *testing.T) {
	job := &domain.Job{
		ID:                "job-1",
		ProjectID:         "project-1",
		EnvironmentID:     "env-1",
		Enabled:           true,
		CacheVersion:      1,
		ExecutionMode:     domain.ExecutionModeHTTP,
		EndpointURL:       "https://example.com/job",
		MaxAttempts:       1,
		TimeoutSecs:       30,
		Version:           1,
		VersionID:         "ver_1",
		VersionPolicy:     domain.VersionPolicyPin,
		CronOverlapPolicy: domain.OverlapPolicyAllow,
	}
	calls := 0
	srv := &Server{
		store: &APIStoreMock{
			GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
				calls++
				return job, nil
			},
		},
		triggerJobCache: newTriggerJobCache(time.Minute),
	}
	t.Cleanup(srv.triggerJobCache.Stop)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, job.ProjectID)
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, job.EnvironmentID)

	got, err := srv.loadRunCreationJob(ctx, job.ID, "test.project_match", "testHandler")
	require.NoError(t, err)
	require.True(t, got.Enabled)

	updated := *job
	updated.Enabled = false
	updated.CacheVersion = 2
	job = &updated
	srv.invalidateJobCaches(ctx, job.ID, job.CacheVersion)

	got, err = srv.loadRunCreationJob(ctx, job.ID, "test.project_match", "testHandler")
	require.NoError(t, err)
	require.False(t, got.Enabled)
	require.Equal(t, 2, calls)
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
