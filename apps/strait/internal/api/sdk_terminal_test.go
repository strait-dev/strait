package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanContinueSDKParentRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.RunStatus
		want   bool
	}{
		{name: "executing", status: domain.StatusExecuting, want: true},
		{name: "waiting", status: domain.StatusWaiting, want: true},
		{name: "queued", status: domain.StatusQueued},
		{name: "completed", status: domain.StatusCompleted},
		{name: "failed", status: domain.StatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, canContinueSDKParentRun(tt.status))
		})
	}
}

func TestSDKTerminal_Complete_Success(t *testing.T) {
	t.Parallel()
	var updateCalled atomic.Bool
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	updatedRun := *run
	updatedRun.Status = domain.StatusCompleted

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id == "run-1" {
				if updateCalled.Load() {
					return &updatedRun, nil
				}
				return run, nil
			}
			return nil, store.ErrRunNotFound
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			updateCalled.Store(true)
			require.Equal(t, "run-1", id)
			require.Equal(t, domain.StatusCompleted,
				to,
			)

			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", `{"result":{"ok":true}}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updateCalled.Load())
}

func TestSDKTerminal_Complete_EmptyResult(t *testing.T) {
	t.Parallel()
	var updateCalled atomic.Bool
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	updatedRun := *run
	updatedRun.Status = domain.StatusCompleted

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if updateCalled.Load() {
				return &updatedRun, nil
			}
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalled.Store(true)
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestSDKTerminal_Complete_ResultExceedsMaxSize(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
	}

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
		MaxResultSize:       10, // 10 bytes max
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Queue:   &mockQueue{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	// Create a result larger than 10 bytes.
	bigResult := `{"result":"` + strings.Repeat("x", 20) + `"}`
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", bigResult)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusRequestEntityTooLarge,

		w.Code)
}

func TestSDKTerminal_Complete_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-999/complete", "run-999", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestSDKTerminal_Complete_Conflict(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusCompleted,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunConflict
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusConflict,
		w.Code,
	)
}

func TestSDKTerminal_Complete_SchemaValidationFailure(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:           "job-1",
		ResultSchema: json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"]}`),
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Send a result that does not match the schema (missing required "count").
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", `{"result":{"name":"test"}}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestSDKTerminal_Complete_PublishesPubSubEvent(t *testing.T) {
	t.Parallel()
	var publishCalled atomic.Bool
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	updatedRun := *run
	updatedRun.Status = domain.StatusCompleted

	var updateDone atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			if updateDone.Load() {
				return &updatedRun, nil
			}
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateDone.Store(true)
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	pub := &mockPublisher{
		publishFn: func(_ context.Context, channel string, _ []byte) error {
			publishCalled.Store(true)
			assert.Equal(
				t, "run:run-1", channel,
			)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, publishCalled.
			Load())
}

func TestSDKTerminal_Fail_Success(t *testing.T) {
	t.Parallel()
	var updateCalled atomic.Bool
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	updatedRun := *run
	updatedRun.Status = domain.StatusFailed
	updatedRun.Error = "something broke"

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			if updateCalled.Load() {
				return &updatedRun, nil
			}
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, _, to domain.RunStatus, fields map[string]any) error {
			updateCalled.Store(true)
			require.Equal(t, "run-1", id)
			require.Equal(t, domain.StatusFailed,
				to)
			require.Equal(t, "something broke",
				fields["error"])

			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/fail", "run-1", `{"error":"something broke"}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updateCalled.Load())
}

func TestSDKTerminal_Fail_MissingError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/fail", "run-1", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestSDKTerminal_Fail_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-999/fail", "run-999", `{"error":"fail"}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestSDKTerminal_Fail_Conflict(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusFailed,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunConflict
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/fail", "run-1", `{"error":"oops"}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusConflict,
		w.Code,
	)
}

func TestSDKTerminal_Fail_PublishesPubSubEvent(t *testing.T) {
	t.Parallel()
	var publishCalled atomic.Bool
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	updatedRun := *run
	updatedRun.Status = domain.StatusFailed

	var updateDone atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			if updateDone.Load() {
				return &updatedRun, nil
			}
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateDone.Store(true)
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	pub := &mockPublisher{
		publishFn: func(_ context.Context, channel string, data []byte) error {
			publishCalled.Store(true)
			assert.Equal(
				t, "run:run-1", channel,
			)

			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err == nil {
				assert.Equal(
					t, "failed", payload["to"])
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/fail", "run-1", `{"error":"boom"}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, publishCalled.
			Load())
}

func TestSDKTerminal_Complete_ResumesWaitingParent(t *testing.T) {
	t.Parallel()
	var parentResumed atomic.Bool
	childRun := &domain.JobRun{
		ID:          "child-1",
		JobID:       "job-1",
		ProjectID:   "proj-1",
		Status:      domain.StatusExecuting,
		ParentRunID: "parent-1",
	}
	parentRun := &domain.JobRun{
		ID:        "parent-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusWaiting,
	}
	completedChild := *childRun
	completedChild.Status = domain.StatusCompleted

	var updateCount atomic.Int32
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			switch id {
			case "child-1":
				if updateCount.Load() > 0 {
					return &completedChild, nil
				}
				return childRun, nil
			case "parent-1":
				return parentRun, nil
			}
			return nil, store.ErrRunNotFound
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			updateCount.Add(1)
			if id == "parent-1" && from == domain.StatusWaiting && to == domain.StatusQueued {
				parentResumed.Store(true)
			}
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/child-1/complete", "child-1", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, parentResumed.
			Load())
}

func TestSDKTerminal_Complete_VeryLargePayloadWithinLimit(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	updatedRun := *run
	updatedRun.Status = domain.StatusCompleted

	var updateDone atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			if updateDone.Load() {
				return &updatedRun, nil
			}
			return run, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateDone.Store(true)
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
		MaxResultSize:       1048576, // 1 MB
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	// 500 bytes is well within 1MB.
	largeResult := `{"result":"` + strings.Repeat("a", 500) + `"}`
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/complete", "run-1", largeResult)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
}
