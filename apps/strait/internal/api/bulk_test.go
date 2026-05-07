package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkTriggerResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Total)
	}
	if resp.Created != 3 {
		t.Fatalf("expected created=3, got %d", resp.Created)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Results))
	}
	var rawResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("invalid raw JSON: %v", err)
	}
	rawResults, ok := rawResp["results"].([]any)
	if !ok || len(rawResults) != 3 {
		t.Fatalf("expected 3 raw results, got %#v", rawResp["results"])
	}
	for idx, rawResult := range rawResults {
		result, ok := rawResult.(map[string]any)
		if !ok {
			t.Fatalf("result %d was not an object: %#v", idx, rawResult)
		}
		if _, ok := result["run_token"]; ok {
			t.Fatalf("bulk trigger result %d must not expose SDK run_token", idx)
		}
	}
	for _, r := range resp.Results {
		if r.ID == "" {
			t.Error("expected non-empty id")
		}
		if r.Status != string(domain.StatusQueued) {
			t.Errorf("expected queued, got %s", r.Status)
		}
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
			if id != job.ID {
				t.Fatalf("GetJob id = %q, want %q", id, job.ID)
			}
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
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected run to be enqueued")
	}
	if captured.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("ExecutionMode = %q, want %q", captured.ExecutionMode, domain.ExecutionModeWorker)
	}
	if captured.QueueName != "priority" {
		t.Fatalf("QueueName = %q, want priority", captured.QueueName)
	}
}

func TestHandleBulkTrigger_WorkerModePropagatesExecutionModeAndQueue(t *testing.T) {
	t.Parallel()

	var captured []*domain.JobRun
	job := testEnabledJob("job-worker")
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = "priority"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			if id != job.ID {
				t.Fatalf("GetJob id = %q, want %q", id, job.ID)
			}
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
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(captured) != 2 {
		t.Fatalf("captured runs = %d, want 2", len(captured))
	}
	for i, run := range captured {
		if run.ExecutionMode != domain.ExecutionModeWorker {
			t.Fatalf("run %d ExecutionMode = %q, want %q", i, run.ExecutionMode, domain.ExecutionModeWorker)
		}
		if run.QueueName != "priority" {
			t.Fatalf("run %d QueueName = %q, want priority", i, run.QueueName)
		}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(received) != 3 {
		t.Fatalf("expected 3 enqueued payloads, got %d", len(received))
	}

	expected := []string{"value1", "value2", "value3"}
	for i, payload := range received {
		var got map[string]string
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatalf("payload %d is invalid JSON: %v", i, err)
		}
		if got["key"] != expected[i] {
			t.Fatalf("payload %d key mismatch: got %q want %q", i, got["key"], expected[i])
		}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkTriggerResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != string(domain.StatusDelayed) {
		t.Fatalf("expected first item status=delayed, got %s", resp.Results[0].Status)
	}
	if resp.Results[1].Status != string(domain.StatusQueued) {
		t.Fatalf("expected second item status=queued, got %s", resp.Results[1].Status)
	}
}

func TestHandleBulkTrigger_EmptyItems(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[]}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
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
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "maximum 500 items") {
		t.Fatalf("expected maximum items error, got %s", w.Body.String())
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBulkTrigger_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
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

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "results") {
		t.Fatalf("expected no partial results in error response, got %s", w.Body.String())
	}
}

func TestHandleBulkTrigger_SingleItem(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[{}]}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkTriggerResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Total != 1 || resp.Created != 1 || len(resp.Results) != 1 {
		t.Fatalf("unexpected response counts: total=%d created=%d results=%d", resp.Total, resp.Created, len(resp.Results))
	}
	if resp.Results[0].Status != string(domain.StatusQueued) {
		t.Fatalf("expected status=queued, got %s", resp.Results[0].Status)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkCancelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Canceled != 3 || resp.Failed != 0 || resp.Total != 3 {
		t.Fatalf("unexpected counters: canceled=%d failed=%d total=%d", resp.Canceled, resp.Failed, resp.Total)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkCancelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Canceled != 1 || resp.Failed != 2 {
		t.Fatalf("expected canceled=1 failed=2, got canceled=%d failed=%d", resp.Canceled, resp.Failed)
	}

	byID := make(map[string]BulkCancelResult, len(resp.Results))
	for _, item := range resp.Results {
		byID[item.ID] = item
	}
	if byID["run-2"].Error != "run not found" {
		t.Fatalf("expected run-2 error run not found, got %q", byID["run-2"].Error)
	}
	if byID["run-3"].Error != "run already in terminal state" {
		t.Fatalf("expected run-3 terminal error, got %q", byID["run-3"].Error)
	}
}

func TestHandleBulkCancel_EmptyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":[]}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleBulkCancel_TooManyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	runIDs := make([]string, 101)
	for i := range runIDs {
		runIDs[i] = fmt.Sprintf("run-%d", i)
	}
	body, err := json.Marshal(map[string]any{"run_ids": runIDs})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", string(body)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "maximum 100 run IDs") {
		t.Fatalf("expected max run IDs error, got %s", w.Body.String())
	}
}

func TestHandleBulkCancel_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":[`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkCancelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Canceled != 0 || resp.Failed != 3 {
		t.Fatalf("expected canceled=0 failed=3, got canceled=%d failed=%d", resp.Canceled, resp.Failed)
	}
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
			if len(parentIDs) != 1 || parentIDs[0] != "run-parent" {
				t.Fatalf("expected parent ID run-parent, got %v", parentIDs)
			}
			childCancelCalled = true
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":["run-parent"]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BulkCancelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Canceled != 1 || resp.Failed != 0 {
		t.Fatalf("expected canceled=1 failed=0, got canceled=%d failed=%d", resp.Canceled, resp.Failed)
	}
	if !childCancelCalled {
		t.Fatal("expected CancelChildRunsByParentIDs to be called")
	}
}
