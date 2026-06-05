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

func TestDeepSec_BulkTriggerRejectsCrossEnvironmentJob(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			job := testEnabledJob(id)
			job.EnvironmentID = "env-staging"
			return job, nil
		},
		CreateBatchOperationFunc: func(context.Context, *domain.BatchOperation) error {
			require.Fail(t,

				"cross-environment bulk trigger must not create a batch")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleBulkTriggerJob(ctx, &BulkTriggerJobInput{
		JobID: "job-1",
		Body:  BulkTriggerRequest{Items: []BulkTriggerItem{{}}},
	})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))
}

func TestDeepSec_BulkCancelAllEnvironmentScopedRequiresJobFilter(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		BulkCancelByFilterFunc: func(context.Context, string, store.BulkCancelFilter, time.Time, string) ([]string, error) {
			require.Fail(t,

				"environment-scoped bulk cancel without job_id must not reach store")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleBulkCancelAll(ctx, &BulkCancelAllInput{Body: BulkCancelAllRequest{Status: domain.StatusQueued}})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusForbidden,
		))
}

func TestDeepSec_BulkCancelAllRejectsCrossEnvironmentJobFilter(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		BulkCancelByFilterFunc: func(context.Context, string, store.BulkCancelFilter, time.Time, string) ([]string, error) {
			require.Fail(t,

				"cross-environment bulk cancel must not reach store")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleBulkCancelAll(ctx, &BulkCancelAllInput{Body: BulkCancelAllRequest{JobID: "job-1"}})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))
}

func TestDeepSec_CreateJobRejectsEnvironmentScopedMismatch(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(context.Context, *domain.Job) error {
			require.Fail(t,

				"environment-scoped create must not persist a job in another environment")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:     "proj-1",
		EnvironmentID: "env-staging",
		Name:          "cross env",
		Slug:          "cross-env",
		EndpointURL:   "https://example.com/run",
	}})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusForbidden,
		))
}

func TestDeepSec_UpdateJobRejectsEnvironmentScopedMove(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			job := testEnabledJob(id)
			job.EnvironmentID = "env-prod"
			return job, nil
		},
		UpdateJobFunc: func(context.Context, *domain.Job) error {
			require.Fail(t,

				"environment-scoped update must not move the job to another environment")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	targetEnv := "env-staging"

	_, err := srv.handleUpdateJob(ctx, &UpdateJobInput{
		JobID: "job-1",
		Body:  UpdateJobRequest{EnvironmentID: &targetEnv},
	})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusForbidden,
		))
}

func TestDeepSec_ListJobsFiltersEnvironmentScopedResults(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListJobsFunc: func(context.Context, string, int, *time.Time) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-prod", ProjectID: "proj-1", EnvironmentID: "env-prod", CreatedAt: time.Now()},
				{ID: "job-staging", ProjectID: "proj-1", EnvironmentID: "env-staging", CreatedAt: time.Now()},
				{ID: "job-unscoped", ProjectID: "proj-1", CreatedAt: time.Now()},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	out, err := srv.handleListJobs(ctx, &ListJobsInput{})
	require.NoError(t, err)

	got := out.Body.Data.([]domain.Job)
	require.False(t, len(got) !=
		1 || got[0].ID !=
		"job-prod")
}

func TestDeepSec_BatchCreateRejectsInvalidCron(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(context.Context, *domain.Job) error {
			require.Fail(t,

				"batch create must not persist invalid cron definitions")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleBatchCreateJobs(ctx, &BatchCreateJobsInput{Body: BatchCreateJobsRequest{
		Jobs: []CreateJobRequest{{
			ProjectID:   "proj-1",
			Name:        "bad cron",
			Slug:        "bad-cron",
			EndpointURL: "https://example.com/run",
			Cron:        "* * *",
		}},
	}})
	require.Error(t, err)
}

func TestDeepSec_CreateJobDependencyRejectsCrossEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			switch id {
			case "job-prod":
				return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				return nil, store.ErrJobNotFound
			}
		},
		CreateJobDependencyFunc: func(context.Context, *domain.JobDependency) error {
			require.Fail(t,

				"cross-environment dependency must not be created")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleCreateJobDependency(ctx, &CreateJobDependencyInput{
		JobID: "job-prod",
		Body:  CreateJobDependencyRequest{DependsOnJobID: "job-staging"},
	})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusBadRequest,
		))
}

func TestDeepSec_UsageDateRangeRejectsMultiYearWindows(t *testing.T) {
	t.Parallel()

	_, _, err := parseDateRangeTyped("2024-01-01", "2026-01-01")
	require.Error(t, err)
}

func TestDeepSec_ProjectScopedAPIKeyCannotMutateOrgSpendingLimit(t *testing.T) {
	t.Parallel()

	enforcer := &mockBillingEnforcer{activeProjectOrgMap: map[string]string{"proj-1": "org-1"}}
	srv := newUsageTestServer(t, enforcer, &mockUsageService{})
	ctx := context.WithValue(context.Background(), ctxScopesKey, []string{domain.ScopeProjectsManage})
	ctx = context.WithValue(ctx, ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	_, err := srv.handleUpdateSpendingLimit(ctx, &UpdateSpendingLimitInput{
		OrgID: "org-1",
		Body:  updateSpendingLimitRequest{LimitMicrousd: 1000, Action: "reject"},
	})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusForbidden,
		))
}
