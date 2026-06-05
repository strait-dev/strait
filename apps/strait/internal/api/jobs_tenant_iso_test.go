package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestTenantIso_Jobs_GetJobHealth_RejectsCrossProject verifies that a caller
// authenticated for project A cannot read job-health stats on a job that
// belongs to project B. The handler must return 404 (not 403) to avoid
// leaking existence.
func TestTenantIso_Jobs_GetJobHealth_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-bbb"}, nil
		},
		GetJobHealthStatsFunc: func(_ context.Context, _ string, _ time.Time) (*store.JobHealthStats, error) {
			require.Fail(t,

				"GetJobHealthStats must not be called for cross-project access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleGetJobHealth(ctx, &GetJobHealthInput{JobID: "job-foreign", Window: "1h"})
	require.True(
		t, isHumaStatusError(err,
			http.StatusNotFound,
		))
}

// TestTenantIso_Jobs_GetJobHealth_RejectsCrossEnv verifies that an
// environment-scoped caller in env-prod cannot read health stats for a job
// that lives in env-staging within the same project.
func TestTenantIso_Jobs_GetJobHealth_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-aaa", EnvironmentID: "env-staging"}, nil
		},
		GetJobHealthStatsFunc: func(_ context.Context, _ string, _ time.Time) (*store.JobHealthStats, error) {
			require.Fail(t,

				"GetJobHealthStats must not be called for cross-env access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleGetJobHealth(ctx, &GetJobHealthInput{JobID: "job-1", Window: "1h"})
	require.True(
		t, isHumaStatusError(err,
			http.StatusNotFound,
		))
}

// TestTenantIso_Jobs_BatchEnable_RejectsForeignIDs ensures that the project_id
// filter on the UPDATE means a caller in project A submitting only
// project-B job IDs receives updated=0; the foreign rows are untouched
// because the WHERE clause excludes them.
func TestTenantIso_Jobs_BatchEnable_RejectsForeignIDs(t *testing.T) {
	t.Parallel()
	var capturedProjectID string
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, _ []string, _ bool, projectID string) (int64, error) {
			capturedProjectID = projectID
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	out, err := srv.handleBatchEnableJobs(ctx, &BatchEnableJobsInput{Body: BatchJobIDsRequest{IDs: []string{"foreign-1", "foreign-2"}}})
	require.NoError(t, err)
	require.EqualValues(t, 0, out.
		Body.Updated,
	)
	require.Equal(t, "proj-aaa",
		capturedProjectID,
	)
}

// TestTenantIso_Jobs_BatchEnable_MixedIDs_OnlyOwnUpdated checks that when a
// mixed batch of caller-owned and foreign IDs is submitted, the store layer
// (which we trust to apply the WHERE project_id) returns the count of own
// rows touched; the handler simply forwards this count.
func TestTenantIso_Jobs_BatchEnable_MixedIDs_OnlyOwnUpdated(t *testing.T) {
	t.Parallel()
	ownProject := "proj-aaa"
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, ids []string, _ bool, projectID string) (int64, error) {
			require.Equal(t, ownProject,
				projectID,
			)

			// Simulated DB: only the first id belongs to ownProject.
			_ = ids
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, ownProject)
	out, err := srv.handleBatchEnableJobs(ctx, &BatchEnableJobsInput{Body: BatchJobIDsRequest{IDs: []string{"own-1", "foreign-1"}}})
	require.NoError(t, err)
	require.EqualValues(t, 1, out.
		Body.Updated,
	)
}

// TestTenantIso_Jobs_BatchDisable_RejectsForeignIDs mirrors the enable case.
func TestTenantIso_Jobs_BatchDisable_RejectsForeignIDs(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, _ []string, enabled bool, _ string) (int64, error) {
			require.False(t, enabled)

			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	out, err := srv.handleBatchDisableJobs(ctx, &BatchDisableJobsInput{Body: BatchJobIDsRequest{IDs: []string{"foreign-1"}}})
	require.NoError(t, err)
	require.EqualValues(t, 0, out.
		Body.Updated,
	)
}

// TestTenantIso_Jobs_BatchDisable_MixedIDs_OnlyOwnUpdated symmetric to enable.
func TestTenantIso_Jobs_BatchDisable_MixedIDs_OnlyOwnUpdated(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, _ []string, _ bool, projectID string) (int64, error) {
			require.Equal(t, "proj-aaa",
				projectID,
			)

			return 1, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	out, err := srv.handleBatchDisableJobs(ctx, &BatchDisableJobsInput{Body: BatchJobIDsRequest{IDs: []string{"own-1", "foreign-1"}}})
	require.NoError(t, err)
	require.EqualValues(t, 1, out.
		Body.Updated,
	)
}

// TestTenantIso_Jobs_BatchEnable_EmptyProjectCtx_Rejected ensures that a
// caller without a project context (e.g. an internal-secret caller missing
// X-Project-Id) is rejected with 400 instead of being able to update
// records across the entire fleet.
func TestTenantIso_Jobs_BatchEnable_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, _ []string, _ bool, _ string) (int64, error) {
			require.Fail(t,

				"BatchUpdateJobsEnabled must not be called without project context")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleBatchEnableJobs(context.Background(), &BatchEnableJobsInput{Body: BatchJobIDsRequest{IDs: []string{"job-1"}}})
	require.True(
		t, isHumaStatusError(err,
			http.StatusBadRequest,
		))
}

// TestTenantIso_Jobs_BatchDisable_EmptyProjectCtx_Rejected symmetric to enable.
func TestTenantIso_Jobs_BatchDisable_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, _ []string, _ bool, _ string) (int64, error) {
			require.Fail(t,

				"BatchUpdateJobsEnabled must not be called without project context")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleBatchDisableJobs(context.Background(), &BatchDisableJobsInput{Body: BatchJobIDsRequest{IDs: []string{"job-1"}}})
	require.True(
		t, isHumaStatusError(err,
			http.StatusBadRequest,
		))
}

// TestTenantIso_Jobs_BatchCreate_RejectsForeignProjectID checks that a
// per-item project-mismatch in the batch payload causes that item to fail
// with a "project not found" error rather than creating a job under a
// foreign project.
func TestTenantIso_Jobs_BatchCreate_RejectsForeignProjectID(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
			require.Fail(t,

				"CreateJob must not be called for foreign project ID")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleBatchCreateJobs(ctx, &BatchCreateJobsInput{Body: BatchCreateJobsRequest{
		Jobs: []CreateJobRequest{
			{
				ProjectID:   "proj-bbb",
				Name:        "Foreign Job",
				Slug:        "foreign-job",
				EndpointURL: "https://example.com/run",
			},
		},
	}})
	require.Error(t, err)

	// The handler returns rawStatusError (400) when no items succeed.

	var rse *rawStatusError
	require.ErrorAs(
		t, err, &rse)
	require.Equal(t, http.StatusBadRequest,

		rse.status,
	)
}
