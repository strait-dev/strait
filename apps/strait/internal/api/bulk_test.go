package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testEnabledJob(id string) *domain.Job {
	return &domain.Job{
		ID:          id,
		ProjectID:   "proj-1",
		Name:        "Test",
		EndpointURL: "https://example.com",
		Enabled:     true,
		TimeoutSecs: 300,
		MaxAttempts: 3,
	}
}

func TestHandleBulkTrigger_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"items":[{},{},{}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,

		w.Code)

	var resp BulkTriggerResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, 3, resp.Total)
	require.Equal(t, 3, resp.Created)
	require.Len(t,
		resp.Results, 3,
	)

	var rawResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&rawResp))

	rawResults, ok := rawResp["results"].([]any)
	require.True(t, ok)
	require.Len(t, rawResults, 3)

	for idx, rawResult := range rawResults {
		result, ok := rawResult.(map[string]any)
		require.True(
			t, ok)

		if _, ok := result["run_token"]; ok {
			require.Failf(t, "test failure",

				"bulk trigger result %d must not expose SDK run_token", idx)
		}
	}
	for _, r := range resp.Results {
		assert.NotEmpty(t, r.ID)
		assert.Equal(
			t, string(domain.
				StatusQueued,
			), r.Status,
		)
	}
}

func TestHandleTriggerJob_WorkerModePropagatesExecutionModeAndQueue(t *testing.T) {
	t.Parallel()

	var captured *domain.JobRun
	job := testEnabledJob("job-worker")
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = "priority"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, job.ID, id)

			return job, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			cp := *run
			captured = &cp
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-worker/trigger", `{"payload":{"ok":true}}`))
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.NotNil(t, captured)
	require.Equal(t, domain.ExecutionModeWorker,

		captured.
			ExecutionMode)
	require.Equal(t, "priority", captured.
		QueueName)
}

func TestHandleBulkTrigger_WorkerModePropagatesExecutionModeAndQueue(t *testing.T) {
	t.Parallel()

	var captured []*domain.JobRun
	job := testEnabledJob("job-worker")
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = "priority"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, job.ID, id)

			return job, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			cp := *run
			captured = append(captured, &cp)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-worker/trigger/bulk", `{"items":[{"payload":{"n":1}},{"payload":{"n":2}}]}`))
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.Len(t,
		captured, 2)

	for _, run := range captured {
		require.Equal(t, domain.ExecutionModeWorker,

			run.
				ExecutionMode)
		require.Equal(t, "priority", run.
			QueueName,
		)
	}
}

func TestHandleBulkTrigger_WithPayloads(t *testing.T) {
	t.Parallel()
	received := make([]json.RawMessage, 0, 3)
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			received = append(received, append(json.RawMessage(nil), run.Payload...))
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"items":[{"payload":{"key":"value1"}},{"payload":{"key":"value2"}},{"payload":{"key":"value3"}}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,

		w.Code)
	require.Len(t,
		received, 3)

	expected := []string{"value1", "value2", "value3"}
	for i, payload := range received {
		var got map[string]string
		require.NoError(t, json.Unmarshal(payload,
			&got))
		require.Equal(t, expected[i],
			got["key"])
	}
}

func TestHandleBulkTrigger_WithScheduledAt(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil)

	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"items":[{"scheduled_at":"%s"},{}]}`, future)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,

		w.Code)

	var resp BulkTriggerResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Len(t,
		resp.Results, 2,
	)
	require.Equal(t, string(domain.
		StatusDelayed,
	), resp.
		Results[0].Status)
	require.Equal(t, string(domain.
		StatusQueued,
	), resp.
		Results[1].Status)
}

func TestHandleBulkTrigger_RejectsOutOfRangeScheduledAtAndTTL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "past scheduled_at",
			body: fmt.Sprintf(`{"items":[{"scheduled_at":"%s"}]}`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)),
			want: "scheduled_at",
		},
		{
			name: "far future scheduled_at",
			body: fmt.Sprintf(`{"items":[{"scheduled_at":"%s"}]}`, time.Now().Add(31*24*time.Hour).UTC().Format(time.RFC3339)),
			want: "30 days",
		},
		{
			name: "negative ttl",
			body: `{"items":[{"ttl_secs":-1}]}`,
			want: "ttl_secs",
		},
		{
			name: "too large ttl",
			body: `{"items":[{"ttl_secs":2592001}]}`,
			want: "ttl_secs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return testEnabledJob(id), nil
				},
			}
			mq := &mockQueue{enqueueFn: func(context.Context, *domain.JobRun) error {
				require.Fail(t,

					"bulk trigger must not enqueue invalid item")
				return nil
			}}
			srv := newTestServer(t, ms, mq, nil)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", tc.body))
			require.False(t, w.Code != http.
				StatusBadRequest &&
				w.Code != http.StatusUnprocessableEntity,
			)
			require.Contains(
				t, w.Body.
					String(), tc.want)
		})
	}
}

func TestHandleBulkTrigger_EmptyItems(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[]}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleBulkTrigger_TooManyItems(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	items := make([]map[string]any, 501)
	for i := range items {
		items[i] = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{"items": items})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", string(body)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "maximum 500 items",
	)
}

func TestHandleBulkTrigger_JobNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[{}]}`))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleBulkTrigger_JobDisabled(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			job := testEnabledJob(id)
			job.Enabled = false
			return job, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[{}]}`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleBulkTrigger_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleBulkTrigger_EnqueueError(t *testing.T) {
	t.Parallel()
	call := 0
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			call++
			if call == 2 {
				return fmt.Errorf("enqueue failed")
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[{},{},{}]}`))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
	require.NotContains(t, w.Body.
		String(), "results")
}

func TestHandleBulkTrigger_SingleItem(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[{}]}`))
	require.Equal(t, http.StatusCreated,

		w.Code)

	var resp BulkTriggerResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.False(t, resp.Total !=
		1 || resp.
		Created !=
		1 || len(resp.Results) != 1)
	require.Equal(t, string(domain.
		StatusQueued,
	), resp.
		Results[0].Status)
}

func TestHandleBulkCancel_Success(t *testing.T) {
	t.Parallel()
	runs := map[string]*domain.JobRun{
		"run-1": {ID: "run-1", Status: domain.StatusExecuting},
		"run-2": {ID: "run-2", Status: domain.StatusExecuting},
		"run-3": {ID: "run-3", Status: domain.StatusExecuting},
	}
	ms := &APIStoreMock{
		GetRunsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
			result := make(map[string]*domain.JobRun)
			for _, id := range ids {
				if r, ok := runs[id]; ok {
					result[id] = r
				}
			}
			return result, nil
		},
		BulkCancelRunsFunc: func(_ context.Context, ids []string, _ time.Time, _ string) ([]store.BulkCancelResult, error) {
			results := make([]store.BulkCancelResult, 0, len(ids))
			for _, id := range ids {
				results = append(results, store.BulkCancelResult{ID: id, Canceled: true})
			}
			return results, nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":["run-1","run-2","run-3"]}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp BulkCancelResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.False(t, resp.Canceled !=
		3 ||
		resp.Failed !=
			0 || resp.Total !=
		3)
}

func TestHandleBulkCancel_PartialFailure(t *testing.T) {
	t.Parallel()
	runs := map[string]*domain.JobRun{
		"run-1": {ID: "run-1", Status: domain.StatusExecuting},
		"run-3": {ID: "run-3", Status: domain.StatusCompleted},
	}
	ms := &APIStoreMock{
		GetRunsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
			result := make(map[string]*domain.JobRun)
			for _, id := range ids {
				if r, ok := runs[id]; ok {
					result[id] = r
				}
			}
			return result, nil
		},
		BulkCancelRunsFunc: func(_ context.Context, ids []string, _ time.Time, _ string) ([]store.BulkCancelResult, error) {
			results := make([]store.BulkCancelResult, 0, len(ids))
			for _, id := range ids {
				results = append(results, store.BulkCancelResult{ID: id, Canceled: true})
			}
			return results, nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":["run-1","run-2","run-3"]}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp BulkCancelResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.False(t, resp.Canceled !=
		1 ||
		resp.Failed !=
			2)

	byID := make(map[string]BulkCancelResult, len(resp.Results))
	for _, item := range resp.Results {
		byID[item.ID] = item
	}
	require.Equal(t, "run not found",
		byID["run-2"].Error,
	)
	require.Equal(t, "run already in terminal state",

		byID["run-3"].Error)
}

func TestHandleBulkCancel_EmptyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":[]}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleBulkCancel_TooManyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	runIDs := make([]string, 101)
	for i := range runIDs {
		runIDs[i] = fmt.Sprintf("run-%d", i)
	}
	body, err := json.Marshal(map[string]any{"run_ids": runIDs})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", string(body)))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "maximum 100 run IDs")
}

func TestHandleBulkCancel_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":[`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleBulkCancel_AllTerminal(t *testing.T) {
	t.Parallel()
	runs := map[string]*domain.JobRun{
		"run-1": {ID: "run-1", Status: domain.StatusCompleted},
		"run-2": {ID: "run-2", Status: domain.StatusFailed},
		"run-3": {ID: "run-3", Status: domain.StatusCanceled},
	}
	ms := &APIStoreMock{
		GetRunsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
			result := make(map[string]*domain.JobRun)
			for _, id := range ids {
				if r, ok := runs[id]; ok {
					result[id] = r
				}
			}
			return result, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":["run-1","run-2","run-3"]}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp BulkCancelResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.False(t, resp.Canceled !=
		0 ||
		resp.Failed !=
			3)
}

func TestHandleBulkCancel_WithChildren(t *testing.T) {
	t.Parallel()
	runs := map[string]*domain.JobRun{
		"run-parent": {ID: "run-parent", Status: domain.StatusExecuting},
	}
	var childCancelCalled bool
	ms := &APIStoreMock{
		GetRunsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
			result := make(map[string]*domain.JobRun)
			for _, id := range ids {
				if r, ok := runs[id]; ok {
					result[id] = r
				}
			}
			return result, nil
		},
		BulkCancelRunsFunc: func(_ context.Context, ids []string, _ time.Time, _ string) ([]store.BulkCancelResult, error) {
			results := make([]store.BulkCancelResult, 0, len(ids))
			for _, id := range ids {
				results = append(results, store.BulkCancelResult{ID: id, Canceled: true})
			}
			return results, nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			require.False(t, len(parentIDs) != 1 ||
				parentIDs[0] != "run-parent")

			childCancelCalled = true
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":["run-parent"]}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp BulkCancelResponse
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.False(t, resp.Canceled !=
		1 ||
		resp.Failed !=
			0)
	require.True(
		t, childCancelCalled,
	)
}
