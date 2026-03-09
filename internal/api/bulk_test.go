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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	for _, r := range resp.Results {
		if r.ID == "" {
			t.Error("expected non-empty id")
		}
		if r.RunToken == "" {
			t.Error("expected non-empty run_token")
		}
		if r.Status != string(domain.StatusQueued) {
			t.Errorf("expected queued, got %s", r.Status)
		}
	}
}

func TestHandleBulkTrigger_WithPayloads(t *testing.T) {
	t.Parallel()
	received := make([]json.RawMessage, 0, 3)
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &mockAPIStore{getJobFn: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", `{"items":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBulkTrigger_TooManyItems(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{getJobFn: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	items := make([]map[string]any, 101)
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
	if !strings.Contains(w.Body.String(), "maximum 100 items") {
		t.Fatalf("expected maximum items error, got %s", w.Body.String())
	}
}

func TestHandleBulkTrigger_JobNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &mockAPIStore{getJobFn: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
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
	ms := &mockAPIStore{getJobFn: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
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
	ms := &mockAPIStore{getJobFn: func(_ context.Context, id string) (*domain.Job, error) { return testEnabledJob(id), nil }}
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
	updates := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if r, ok := runs[id]; ok {
				return r, nil
			}
			return nil, fmt.Errorf("not found")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			if to != domain.StatusCanceled {
				t.Fatalf("expected transition to canceled, got %s", to)
			}
			updates++
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
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
	if updates != 3 {
		t.Fatalf("expected 3 status updates, got %d", updates)
	}
}

func TestHandleBulkCancel_PartialFailure(t *testing.T) {
	t.Parallel()
	runs := map[string]*domain.JobRun{
		"run-1": {ID: "run-1", Status: domain.StatusExecuting},
		"run-3": {ID: "run-3", Status: domain.StatusCompleted},
	}
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if r, ok := runs[id]; ok {
				return r, nil
			}
			return nil, fmt.Errorf("not found")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
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
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", `{"run_ids":[]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBulkCancel_TooManyRunIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

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
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if r, ok := runs[id]; ok {
				return r, nil
			}
			return nil, fmt.Errorf("not found")
		},
		listChildRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
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
	updatedIDs := make([]string, 0, 2)
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if r, ok := runs[id]; ok {
				return r, nil
			}
			return nil, fmt.Errorf("not found")
		},
		updateRunStatusFn: func(_ context.Context, id string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			if to != domain.StatusCanceled {
				t.Fatalf("expected canceled status, got %s", to)
			}
			updatedIDs = append(updatedIDs, id)
			return nil
		},
		listChildRunsFn: func(_ context.Context, parentRunID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			if parentRunID != "run-parent" {
				t.Fatalf("unexpected parent run ID: %s", parentRunID)
			}
			return []domain.JobRun{
				{ID: "run-child-1", Status: domain.StatusExecuting},
				{ID: "run-child-2", Status: domain.StatusCompleted},
			}, nil
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
	if len(updatedIDs) != 2 {
		t.Fatalf("expected parent + one child cancellation updates, got %d", len(updatedIDs))
	}

	seen := map[string]bool{}
	for _, id := range updatedIDs {
		seen[id] = true
	}
	if !seen["run-parent"] {
		t.Fatal("expected parent run cancellation")
	}
	if !seen["run-child-1"] {
		t.Fatal("expected non-terminal child run cancellation")
	}
	if seen["run-child-2"] {
		t.Fatal("did not expect terminal child run cancellation")
	}
}
