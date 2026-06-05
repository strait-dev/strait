package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestTenantIso_JobVersions_List_RejectsCrossProject ensures listing
// versions for a job that belongs to another project returns 404 instead
// of leaking the version history.
func TestTenantIso_JobVersions_List_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-bbb"}, nil
		},
		ListJobVersionsByJobFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobVersion, error) {
			require.Fail(t,

				"ListJobVersionsByJob must not be called for cross-project access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleListJobVersions(ctx, &ListJobVersionsInput{JobID: "job-foreign"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))

}

// TestTenantIso_JobVersions_List_RejectsCrossEnv covers an env-scoped
// caller that should not see versions from a different environment in
// the same project.
func TestTenantIso_JobVersions_List_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-aaa", EnvironmentID: "env-staging"}, nil
		},
		ListJobVersionsByJobFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobVersion, error) {
			require.Fail(t,

				"ListJobVersionsByJob must not be called for cross-env access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleListJobVersions(ctx, &ListJobVersionsInput{JobID: "job-1"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))

}

// TestTenantIso_JobVersions_Get_RejectsCrossProject ensures the per-version
// lookup also enforces project membership of the parent job, even when the
// (job_id, version_id) pair otherwise matches.
func TestTenantIso_JobVersions_Get_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobVersionByVersionIDFunc: func(_ context.Context, _ string) (*domain.JobVersion, error) {
			return &domain.JobVersion{JobID: "job-foreign", VersionID: "ver-1"}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-bbb"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleGetJobVersion(ctx, &GetJobVersionInput{JobID: "job-foreign", VersionID: "ver-1"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))

}

// TestTenantIso_JobVersions_Get_RejectsCrossEnv covers env scoping on the
// per-version lookup path.
func TestTenantIso_JobVersions_Get_RejectsCrossEnv(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobVersionByVersionIDFunc: func(_ context.Context, _ string) (*domain.JobVersion, error) {
			return &domain.JobVersion{JobID: "job-1", VersionID: "ver-1"}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-aaa", EnvironmentID: "env-staging"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	_, err := srv.handleGetJobVersion(ctx, &GetJobVersionInput{JobID: "job-1", VersionID: "ver-1"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))

}
