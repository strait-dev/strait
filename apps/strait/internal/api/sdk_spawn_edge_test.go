package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Await completion tests.

func TestHandleSDKSpawn_AwaitCompletion_TransitionsParent(t *testing.T) {
	t.Parallel()
	var statusUpdated atomic.Bool
	var createdTrigger atomic.Bool

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: "child"}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			if from == domain.StatusExecuting && to == domain.StatusWaiting {
				statusUpdated.Store(true)
			}
			return nil
		},
		CreateEventTriggerFunc: func(_ context.Context, _ *domain.EventTrigger) error {
			createdTrigger.Store(true)
			return nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-1","await_completion":true,"await_timeout_secs":300}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	assert.True(t,
		statusUpdated.Load())
	assert.True(t,
		createdTrigger.
			Load())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(
		t, true, resp["await_completion"])

	if _, ok := resp["await_event_key"]; !ok {
		assert.Fail(t,

			"response should include await_event_key")
	}
}

func TestHandleSDKSpawn_AwaitCompletion_RejectsTimeoutAboveMaximum(t *testing.T) {
	t.Parallel()
	var enqueued atomic.Bool
	var statusUpdated atomic.Bool
	var createdTrigger atomic.Bool

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: "child"}, nil
		},
		UpdateRunStatusFunc: func(context.Context, string, domain.RunStatus, domain.RunStatus, map[string]any) error {
			statusUpdated.Store(true)
			return nil
		},
		CreateEventTriggerFunc: func(context.Context, *domain.EventTrigger) error {
			createdTrigger.Store(true)
			return nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			enqueued.Store(true)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-1","await_completion":true,"await_timeout_secs":2592001}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.False(t, enqueued.Load() || statusUpdated.
		Load() || createdTrigger.
		Load())
}

func TestHandleSDKSpawn_NoAwait_DoesNotTransitionParent(t *testing.T) {
	t.Parallel()
	var statusUpdated atomic.Bool

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: "child"}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			statusUpdated.Store(true)
			return nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	assert.False(
		t, statusUpdated.
			Load())
}

func TestHandleSDKSpawn_AwaitCompletion_ParentNotExecuting(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			// Parent is already waiting (e.g., from a previous spawn).
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusWaiting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: "child"}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			assert.Fail(t,

				"should not update status when parent is not executing")
			return nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-1","await_completion":true}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	// Should still create the child run, but not transition parent.
}

// Cross-project spawn tests.

func TestHandleSDKSpawn_CrossProject_RequiresTargetKey(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// project_id differs from parent's project, but no target_api_key.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleSDKSpawn_CrossProject_InvalidKey(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return nil, errors.New("key not found")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_invalid123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestHandleSDKSpawn_CrossProject_RevokedKey(t *testing.T) {
	t.Parallel()
	revokedAt := time.Now().Add(-time.Hour)
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-target", RevokedAt: &revokedAt}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_revoked123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestHandleSDKSpawn_CrossProject_ExpiredKey(t *testing.T) {
	t.Parallel()
	expiredAt := time.Now().Add(-time.Hour)
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-target", ExpiresAt: &expiredAt}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_expired123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestHandleSDKSpawn_CrossProject_WrongProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			// Key belongs to a different project than specified.
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-other"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_wrong123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
}

func TestHandleSDKSpawn_CrossProject_TargetKeyRequiresTriggerScope(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-target", Scopes: []string{domain.ScopeJobsRead}}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, _, _ string) (*domain.Job, error) {
			require.Fail(t,

				"job lookup should not run when target key lacks jobs:trigger")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_readonly123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
}

func TestHandleSDKSpawn_CrossProject_TargetKeyEnvironmentMustMatchJob(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID:            "key-1",
				ProjectID:     "proj-target",
				EnvironmentID: "env-prod",
				Scopes:        []string{domain.ScopeJobsTrigger},
			}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-target", ProjectID: projectID, Slug: "child", EnvironmentID: "env-staging"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			require.Fail(t,

				"enqueue should not run for a target-key environment mismatch")
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_env123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
}

func TestHandleSDKSpawn_CrossProject_ValidKey(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-target"}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			require.Equal(t, "proj-target",
				projectID,
			)

			return &domain.Job{ID: "job-target", ProjectID: projectID, Slug: "child"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			require.Equal(t, "proj-target",
				run.ProjectID,
			)

			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_valid123"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

func TestHandleSDKSpawn_CrossProject_TargetKeyUsesAuthCache(t *testing.T) {
	t.Parallel()

	var keyLookups atomic.Int64
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-source", Status: domain.StatusExecuting}, nil
		},
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			keyLookups.Add(1)
			return &domain.APIKey{
				ID:           "key-1",
				ProjectID:    "proj-target",
				Scopes:       []string{domain.ScopeJobsTrigger},
				CacheVersion: 6,
			}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-target", ProjectID: projectID, Slug: "child"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil)
	srv.apiKeyCache = newAPIKeyCache(time.Minute)

	for range 2 {
		w := httptest.NewRecorder()
		r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
			`{"job_slug":"child","project_id":"proj-target","target_api_key":"strait_cached123"}`)
		srv.ServeHTTP(w, r)
		require.Equal(t, http.StatusCreated,
			w.Code,
		)
	}
	require.EqualValues(t, 1, keyLookups.
		Load())
}

func TestHandleSDKSpawn_SameProject_NoKeyNeeded(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: projectID, Slug: "child"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent",
		`{"job_slug":"child","project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

func TestHandleSDKSpawn_ParentRunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-gone/spawn", "run-gone",
		`{"job_slug":"child","project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
}
