package worker

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
)

func TestResolveJobForRun_EmptyPolicyFallsToPin(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 5, VersionID: "ver_v5",
				VersionPolicy: "", // empty policy = pin
				EndpointURL:   "https://v5.example.com",
				MaxAttempts:   3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v2",
				EndpointURL: "https://v2.example.com",
				MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 2, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v2.example.com" {
		t.Errorf("expected v2 endpoint (pin behavior), got %s", job.EndpointURL)
	}
	if run.JobVersion != 2 {
		t.Errorf("expected run version to stay 2, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_Latest_UpdatesVersionID(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 4, VersionID: "ver_v4",
				VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL:   "https://v4.example.com",
				MaxAttempts:   3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1",
		JobVersion: 1, JobVersionID: "ver_v1",
		Status: domain.StatusDequeued,
	}

	_, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.JobVersion != 4 {
		t.Errorf("expected run version = 4, got %d", run.JobVersion)
	}
	if run.JobVersionID != "ver_v4" {
		t.Errorf("expected run version_id = ver_v4, got %s", run.JobVersionID)
	}
}

func TestResolveJobForRun_Minor_Compatible_UpdatesVersionID(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3",
				VersionPolicy:       domain.VersionPolicyMinor,
				BackwardsCompatible: true,
				EndpointURL:         "https://v3.example.com",
				MaxAttempts:         3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1",
		JobVersion: 1, JobVersionID: "ver_v1",
		Status: domain.StatusDequeued,
	}

	_, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.JobVersion != 3 {
		t.Errorf("expected version upgrade to 3, got %d", run.JobVersion)
	}
	if run.JobVersionID != "ver_v3" {
		t.Errorf("expected version_id upgrade to ver_v3, got %s", run.JobVersionID)
	}
}

func TestResolveJobForRun_GetJobError(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, errors.New("db error")
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	_, err := e.resolveJobForRun(context.Background(), run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveJobForRun_GetJobAtVersionError(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 5, VersionID: "ver_v5",
				VersionPolicy: domain.VersionPolicyPin,
				EndpointURL:   "https://v5.example.com",
				MaxAttempts:   3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, _ int) (*domain.Job, error) {
			return nil, errors.New("version not found")
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 2, Status: domain.StatusDequeued}

	_, err := e.resolveJobForRun(context.Background(), run)
	if err == nil {
		t.Fatal("expected error when versioned job not found")
	}
}

func TestResolveJobForRun_Pin_MultipleVersionGap(t *testing.T) {
	t.Parallel()

	var requestedVersion int
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 10, VersionID: "ver_v10",
				VersionPolicy: domain.VersionPolicyPin,
				EndpointURL:   "https://v10.example.com",
				MaxAttempts:   3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			requestedVersion = v
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v1",
				EndpointURL: "https://v1.example.com",
				MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestedVersion != 1 {
		t.Errorf("expected GetJobAtVersion called with version 1, got %d", requestedVersion)
	}
	if job.EndpointURL != "https://v1.example.com" {
		t.Errorf("expected v1 endpoint, got %s", job.EndpointURL)
	}
}

func TestResolveJobForRun_Minor_NotBackwardsCompatible_FallsToPin(t *testing.T) {
	t.Parallel()

	getJobAtVersionCalled := false
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3",
				VersionPolicy:       domain.VersionPolicyMinor,
				BackwardsCompatible: false,
				EndpointURL:         "https://v3.example.com",
				MaxAttempts:         3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			getJobAtVersionCalled = true
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v1",
				EndpointURL: "https://v1.example.com",
				MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !getJobAtVersionCalled {
		t.Fatal("expected GetJobAtVersion to be called for incompatible minor policy")
	}
	if job.EndpointURL != "https://v1.example.com" {
		t.Errorf("expected pinned v1 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 1 {
		t.Errorf("expected version to remain 1, got %d", run.JobVersion)
	}
}
