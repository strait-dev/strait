package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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
			t.Fatal("cross-environment bulk trigger must not create a batch")
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
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for cross-environment bulk trigger, got %v", err)
	}
}

func TestDeepSec_BulkCancelAllEnvironmentScopedRequiresJobFilter(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		BulkCancelByFilterFunc: func(context.Context, string, store.BulkCancelFilter, time.Time, string) ([]string, error) {
			t.Fatal("environment-scoped bulk cancel without job_id must not reach store")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleBulkCancelAll(ctx, &BulkCancelAllInput{Body: BulkCancelAllRequest{Status: domain.StatusQueued}})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for unscoped environment bulk cancel, got %v", err)
	}
}

func TestDeepSec_BulkCancelAllRejectsCrossEnvironmentJobFilter(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		BulkCancelByFilterFunc: func(context.Context, string, store.BulkCancelFilter, time.Time, string) ([]string, error) {
			t.Fatal("cross-environment bulk cancel must not reach store")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleBulkCancelAll(ctx, &BulkCancelAllInput{Body: BulkCancelAllRequest{JobID: "job-1"}})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for cross-environment job filter, got %v", err)
	}
}

func TestDeepSec_CreateJobRejectsEnvironmentScopedMismatch(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(context.Context, *domain.Job) error {
			t.Fatal("environment-scoped create must not persist a job in another environment")
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
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for cross-environment create, got %v", err)
	}
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
			t.Fatal("environment-scoped update must not move the job to another environment")
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
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for cross-environment update, got %v", err)
	}
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
	if err != nil {
		t.Fatalf("handleListJobs error = %v", err)
	}
	got := out.Body.Data.([]domain.Job)
	if len(got) != 1 || got[0].ID != "job-prod" {
		t.Fatalf("filtered jobs = %+v, want only job-prod", got)
	}
}

func TestDeepSec_BatchCreateRejectsInvalidCron(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(context.Context, *domain.Job) error {
			t.Fatal("batch create must not persist invalid cron definitions")
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
	if err == nil {
		t.Fatal("expected batch create to reject invalid cron")
	}
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
			t.Fatal("cross-environment dependency must not be created")
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
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400 for cross-environment dependency, got %v", err)
	}
}

func TestDeepSec_UsageDateRangeRejectsMultiYearWindows(t *testing.T) {
	t.Parallel()

	_, _, err := parseDateRangeTyped("2024-01-01", "2026-01-01")
	if err == nil {
		t.Fatal("expected multi-year usage date range to be rejected")
	}
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
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for project-scoped billing mutation, got %v", err)
	}
}
