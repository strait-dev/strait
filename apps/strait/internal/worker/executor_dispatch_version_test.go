package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExecute_UsesVersionedJobConfig(t *testing.T) {
	t.Parallel()

	// v1 endpoint (the one the run was enqueued with)
	v1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"v1"}`))
	}))
	defer v1Server.Close()

	// v2 endpoint (the current/live endpoint -- should not be used)
	v2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("v2 endpoint was called -- executor should have used v1")
		w.WriteHeader(http.StatusOK)
	}))
	defer v2Server.Close()

	store := &mockExecutorStore{}

	// GetJob returns the "current" v2 config
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(v2Server.URL, 1, 5), nil
	}

	// GetJobAtVersion returns the v1 snapshot
	store.getJobAtVersionFn = func(_ context.Context, _ string, version int) (*domain.Job, error) {
		if version == 1 {
			return testJob(v1Server.URL, 1, 5), nil
		}
		return testJob(v2Server.URL, 1, 5), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, v1Server.Client())

	run := testRun(1)
	run.JobVersion = 1

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecute_FallsBackToLiveJob(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &mockExecutorStore{}

	// GetJob returns live config (the fallback)
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(server.URL, 1, 5), nil
	}

	// GetJobAtVersion delegates to GetJob (simulating no snapshot exists)
	store.getJobAtVersionFn = func(ctx context.Context, jobID string, _ int) (*domain.Job, error) {
		return store.GetJob(ctx, jobID)
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	run.JobVersion = 1

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecute_VersionedConfig_PreservesTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &mockExecutorStore{}

	// Live job has timeout=1s, versioned snapshot has timeout=300s
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		return testJob(server.URL, 1, 1), nil
	}
	store.getJobAtVersionFn = func(_ context.Context, _ string, version int) (*domain.Job, error) {
		if version == 1 {
			return testJob(server.URL, 1, 300), nil // original generous timeout
		}
		return testJob(server.URL, 1, 1), nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, server.Client())

	run := testRun(1)
	run.JobVersion = 1

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s (v1 timeout should be 300s not 1s)", calls[1].to, domain.StatusCompleted)
	}
}

func TestResolveJobForRun_Pin(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyPin,
				EndpointURL: "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v1",
				EndpointURL: "https://v1.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v1.example.com" {
		t.Fatalf("expected v1 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 1 {
		t.Fatalf("expected run version to stay 1, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_Latest(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL: "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v3.example.com" {
		t.Fatalf("expected v3 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 3 {
		t.Fatalf("expected run version upgraded to 3, got %d", run.JobVersion)
	}
	if run.JobVersionID != "ver_v3" {
		t.Fatalf("expected run version_id upgraded to ver_v3, got %s", run.JobVersionID)
	}
}

func TestResolveJobForRun_Minor_Compatible(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyMinor,
				BackwardsCompatible: true,
				EndpointURL:         "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v3.example.com" {
		t.Fatalf("expected v3 endpoint, got %s", job.EndpointURL)
	}
	if run.JobVersion != 3 {
		t.Fatalf("expected run version upgraded to 3, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_Minor_Incompatible(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 3, VersionID: "ver_v3", VersionPolicy: domain.VersionPolicyMinor,
				BackwardsCompatible: false,
				EndpointURL:         "https://v3.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
		getJobAtVersionFn: func(_ context.Context, _ string, v int) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: v, VersionID: "ver_v1",
				EndpointURL: "https://v1.example.com", MaxAttempts: 3, TimeoutSecs: 30,
			}, nil
		},
	}
	e := newTestExecutor(t, ms, nil, 0, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", JobVersion: 1, Status: domain.StatusDequeued}

	job, err := e.resolveJobForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.EndpointURL != "https://v1.example.com" {
		t.Fatalf("expected v1 endpoint (no upgrade), got %s", job.EndpointURL)
	}
	if run.JobVersion != 1 {
		t.Fatalf("expected run version to stay 1, got %d", run.JobVersion)
	}
}

func TestResolveJobForRun_SameVersion(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{
				ID: "job-1", Version: 2, VersionID: "ver_v2", VersionPolicy: domain.VersionPolicyLatest,
				EndpointURL: "https://v2.example.com", MaxAttempts: 3, TimeoutSecs: 30,
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
		t.Fatalf("expected current endpoint, got %s", job.EndpointURL)
	}
}

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
