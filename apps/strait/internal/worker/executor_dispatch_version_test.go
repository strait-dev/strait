package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Fail(t,

			"v2 endpoint was called -- executor should have used v1")
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
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,

		calls[1].to)

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
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,

		calls[1].to)

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
	require.Len(t, calls,
		2)
	require.Equal(t,
		domain.StatusCompleted,

		calls[1].to)

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
	require.NoError(
		t, err)
	require.Equal(t,
		"https://v1.example.com",

		job.EndpointURL)
	require.EqualValues(t, 1, run.JobVersion)

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
	require.NoError(
		t, err)
	require.Equal(t,
		"https://v3.example.com",

		job.EndpointURL)
	require.EqualValues(t, 3, run.JobVersion)
	require.Equal(t,
		"ver_v3",
		run.JobVersionID,
	)

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
	require.NoError(
		t, err)
	require.Equal(t,
		"https://v3.example.com",

		job.EndpointURL)
	require.EqualValues(t, 3, run.JobVersion)

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
	require.NoError(
		t, err)
	require.Equal(t,
		"https://v1.example.com",

		job.EndpointURL)
	require.EqualValues(t, 1, run.JobVersion)

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
	require.NoError(
		t, err)
	require.Equal(t,
		"https://v2.example.com",

		job.EndpointURL)

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
	require.NoError(
		t, err)
	assert.Equal(t,
		"https://v2.example.com",

		job.EndpointURL)
	assert.EqualValues(t, 2, run.JobVersion)

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
	require.NoError(
		t, err)
	assert.EqualValues(t, 4, run.JobVersion)
	assert.Equal(t,
		"ver_v4",
		run.JobVersionID,
	)

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
	require.NoError(
		t, err)
	assert.EqualValues(t, 3, run.JobVersion)
	assert.Equal(t,
		"ver_v3",
		run.JobVersionID,
	)

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
	require.Error(t,
		err)

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
	require.Error(t,
		err)

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
	require.NoError(
		t, err)
	assert.EqualValues(t, 1, requestedVersion)
	assert.Equal(t,
		"https://v1.example.com",

		job.EndpointURL)

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
	require.NoError(
		t, err)
	require.True(t,
		getJobAtVersionCalled,
	)
	assert.Equal(t,
		"https://v1.example.com",

		job.EndpointURL)
	assert.EqualValues(t, 1, run.JobVersion)

}
