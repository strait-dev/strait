package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestHandleUpdateJob_Success(t *testing.T) {
	t.Parallel()
	var updated *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old Name", Slug: "job", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			updated = job
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"name":"Updated Name"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updated == nil {
		t.Fatal("expected UpdateJob to be called")
	}
	if updated.Name != "Updated Name" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["name"] != "Updated Name" {
		t.Fatalf("expected response name Updated Name, got %v", resp["name"])
	}
}

func TestHandleUpdateJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/missing", `{"name":"Updated Name"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateJob_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old", EndpointURL: "https://example.com"}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"name":`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateJob_InvalidCron(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old", EndpointURL: "https://example.com"}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"cron":"bad cron"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old", EndpointURL: "https://example.com"}, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			return errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"name":"Updated"}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleCreateJob_MissingFields_ProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"","name":"Job","slug":"job","endpoint_url":"https://example.com"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleCreateJob_InvalidURL(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"not-a-url"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleCreateJob_InvalidCron(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"https://example.com","cron":"bad cron"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
			return errors.New("insert failed")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"https://example.com"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleCreateJob_DefaultValues(t *testing.T) {
	t.Parallel()
	var got *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			got = job
			job.ID = "job-created"
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"https://example.com"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected CreateJob to be called")
	}
	if got.MaxAttempts != 3 {
		t.Fatalf("expected default max_attempts=3, got %d", got.MaxAttempts)
	}
	if got.TimeoutSecs != 300 {
		t.Fatalf("expected default timeout_secs=300, got %d", got.TimeoutSecs)
	}
}

func TestHandleDeleteJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
		DeleteJobFunc: func(_ context.Context, _ string) error {
			return store.ErrJobNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-404", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		DeleteJobFunc: func(_ context.Context, _ string) error {
			return errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-500", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleCancelRun_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-404", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleCancelRun_TerminalState(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-1", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCancelRun_UpdateError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunConflict
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-1", ""))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandleCancelRun_PropagatesChildren(t *testing.T) {
	t.Parallel()
	getRunCalls := 0
	updates := make(map[string]domain.RunStatus)
	var bulkCancelParentIDs []string

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCanceled}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			updates[id] = to
			return nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			bulkCancelParentIDs = append(bulkCancelParentIDs, parentIDs...)
			// Simulate canceling 1 non-terminal child (child-running), skipping the terminal one (child-done).
			if len(parentIDs) > 0 && parentIDs[0] == "run-parent" {
				return 1, nil
			}
			return 0, nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, cursor *time.Time) ([]domain.JobRun, error) {
			if cursor != nil {
				return nil, nil
			}
			return []domain.JobRun{
				{ID: "child-running", ParentRunID: "run-parent", Status: domain.StatusCanceled, CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{ID: "child-done", ParentRunID: "run-parent", Status: domain.StatusCompleted, CreatedAt: time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-parent", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updates["run-parent"] != domain.StatusCanceled {
		t.Fatalf("expected parent run to be canceled, got %q", updates["run-parent"])
	}
	if len(bulkCancelParentIDs) == 0 || bulkCancelParentIDs[0] != "run-parent" {
		t.Fatalf("expected CancelChildRunsByParentIDs called with run-parent, got %v", bulkCancelParentIDs)
	}
}

func TestHandleCancelRun_PropagatesChildren_MultiPage(t *testing.T) {
	t.Parallel()
	getRunCalls := 0
	bulkCancelCalls := 0
	parentListCalls := 0

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCanceled}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			bulkCancelCalls++
			// First call: cancel children of run-parent (3 children)
			if bulkCancelCalls == 1 {
				return 3, nil
			}
			// Second call: no grandchildren
			return 0, nil
		},
		ListChildRunsFunc: func(_ context.Context, parentRunID string, _ int, cursor *time.Time) ([]domain.JobRun, error) {
			// Only the parent has children; child runs have no grandchildren.
			if parentRunID != "run-parent" {
				return nil, nil
			}
			parentListCalls++
			switch parentListCalls {
			case 1:
				return []domain.JobRun{
					{ID: "child-1", Status: domain.StatusCanceled, CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
					{ID: "child-2", Status: domain.StatusCanceled, CreatedAt: time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
				}, nil
			case 2:
				return []domain.JobRun{
					{ID: "child-3", Status: domain.StatusCanceled, CreatedAt: time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC)},
				}, nil
			default:
				return nil, nil
			}
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-parent", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if bulkCancelCalls < 1 {
		t.Fatalf("expected at least 1 CancelChildRunsByParentIDs call, got %d", bulkCancelCalls)
	}
}

func TestHandleTriggerJob_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-missing/trigger", `{}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleTriggerJob_IdempotencyHit(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if jobID != "job-123" || key != "same-key" {
				t.Fatalf("unexpected idempotency lookup args: %s %s", jobID, key)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "same-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (idempotent hit), got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected enqueue to be skipped for idempotency hit")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["id"] != "run-existing" {
		t.Fatalf("expected existing run id, got %v", resp["id"])
	}
	if _, ok := resp["run_token"]; ok {
		t.Fatal("did not expect run_token for idempotency hit")
	}
}

func TestHandleTriggerJob_DelayedSchedule(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 120}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	future := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	body := `{"scheduled_at":"` + future + `"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueued.Status != domain.StatusDelayed {
		t.Fatalf("expected delayed status, got %s", enqueued.Status)
	}
}

func TestHandleTriggerJob_PayloadValidationEnabled(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				Enabled:       true,
				TimeoutSecs:   120,
				PayloadSchema: json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}

	enqueued := false
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"name":"leo"}}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !enqueued {
		t.Fatal("expected run to be enqueued")
	}
}

func TestHandleTriggerJob_PayloadValidationRejectsInvalidPayload(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				Enabled:       true,
				TimeoutSecs:   120,
				PayloadSchema: json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
			}, nil
		},
	}

	enqueued := false
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"age":12}}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected run to not be enqueued")
	}
}

func TestHandleTriggerJob_EnqueueError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 120}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue down")
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleTriggerJob_ImmediateBatchFlushPreservesWorkerModeQueue(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:              id,
				ProjectID:       "proj-1",
				Enabled:         true,
				TimeoutSecs:     120,
				BatchWindowSecs: 60,
				BatchMaxSize:    2,
				ExecutionMode:   domain.ExecutionModeWorker,
				Queue:           "critical-workers",
			}, nil
		},
		InsertBatchBufferItemFunc: func(_ context.Context, _ *domain.BatchBufferItem) error {
			return nil
		},
		CountBatchBufferItemsFunc: func(_ context.Context, _, _ string) (int, error) {
			return 2, nil
		},
		DrainBatchBufferFunc: func(_ context.Context, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error) {
			if jobID != "job-123" || batchKey != "batch-a" || limit != 2 {
				t.Fatalf("unexpected drain args: job=%q batch=%q limit=%d", jobID, batchKey, limit)
			}
			return []domain.BatchBufferItem{
				{Payload: json.RawMessage(`{"n":1}`)},
				{Payload: json.RawMessage(`{"n":2}`)},
			}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"batch_key":"batch-a","payload":{"n":2}}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued == nil {
		t.Fatal("expected immediate batch run to be enqueued")
	}
	if enqueued.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("execution mode = %q, want worker", enqueued.ExecutionMode)
	}
	if enqueued.QueueName != "critical-workers" {
		t.Fatalf("queue = %q, want critical-workers", enqueued.QueueName)
	}
}

func TestValidateURL_ValidHTTPS(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("https://example.com", false); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateURL_ValidHTTP(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("http://example.com", false); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateURL_InvalidScheme(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("ftp://example.com", false); err == nil {
		t.Fatal("expected error for invalid scheme")
	}
}

func TestValidateURL_NoHost(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("http://", false); err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestValidateURL_LoopbackIP(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("http://127.0.0.1", false); err == nil {
		t.Fatal("expected error for loopback IP")
	}
}

func TestValidateURL_PrivateIP(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("http://192.168.1.1", false); err == nil {
		t.Fatal("expected error for private IP")
	}
}

func TestValidateURL_AllowPrivateEndpointsAllowsLoopback(t *testing.T) {
	globalAllowPrivateEndpoints.Store(true)
	t.Cleanup(func() { globalAllowPrivateEndpoints.Store(false) })

	if err := validateURL("http://127.0.0.1:49152/webhook"); err != nil {
		t.Fatalf("expected loopback URL to be allowed, got %v", err)
	}
}

func TestValidateURLWithTLS_AllowPrivateEndpointsRespectsTLS(t *testing.T) {
	globalAllowPrivateEndpoints.Store(true)
	t.Cleanup(func() { globalAllowPrivateEndpoints.Store(false) })

	if err := validateURLWithTLS("http://127.0.0.1:49152/webhook", true); err == nil {
		t.Fatal("expected TLS requirement to reject http URL")
	}
	if err := validateURLWithTLS("http://127.0.0.1:49152/webhook", false); err != nil {
		t.Fatalf("expected loopback URL to be allowed without TLS requirement, got %v", err)
	}
}

func TestValidateURL_InvalidURL(t *testing.T) {
	t.Parallel()
	if err := validateURLWithAllowPrivate("://bad", false); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestValidateURL_ErrorCasing(t *testing.T) {
	t.Parallel()
	err := validateURLWithAllowPrivate("ftp://example.com", false)
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	msg := err.Error()
	if msg[:3] != "url" {
		t.Fatalf("expected error message to start with lowercase 'url', got %q", msg)
	}
	if msg[3] == ' ' {
		// Good: "url must use http or https scheme"
	} else {
		t.Fatalf("expected space after 'url', got %q", msg)
	}
}

func TestHandleStats_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/stats", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleListChildRuns_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-parent/children", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleListChildRuns_SuccessBody(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{{ID: "run-child-1"}, {ID: "run-child-2"}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-parent/children", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "run-child-1") {
		t.Fatalf("expected response to include child run IDs, got %s", w.Body.String())
	}
}

func TestHandleGetJob_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/job-123", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleListJobs_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListJobsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/jobs/", "", "proj-1"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetRun_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-123", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleUpdateJob_AllFields(t *testing.T) {
	t.Parallel()
	name := "new name"
	slug := "new-slug"
	desc := "new description"
	cronExpr := "0 * * * *"
	schema := json.RawMessage(`{"type":"object"}`)
	endpoint := "https://api.example.com/hook"
	maxAttempts := 7
	timeout := 42
	enabled := false

	var updated *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				Name:          "old",
				Slug:          "old",
				Description:   "old",
				Cron:          "",
				EndpointURL:   "https://example.com",
				MaxAttempts:   3,
				TimeoutSecs:   300,
				Enabled:       true,
				PayloadSchema: json.RawMessage(`{"type":"null"}`),
			}, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			updated = job
			return nil
		},
	}

	body := `{"name":"` + name + `","slug":"` + slug + `","description":"` + desc + `","cron":"` + cronExpr + `","payload_schema":{"type":"object"},"endpoint_url":"` + endpoint + `","max_attempts":7,"timeout_secs":42,"enabled":false}`

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-all", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updated == nil {
		t.Fatal("expected updated job")
	}
	if updated.Name != name || updated.Slug != slug || updated.Description != desc || updated.Cron != cronExpr {
		t.Fatalf("unexpected string field values: %+v", updated)
	}
	if updated.EndpointURL != endpoint || updated.MaxAttempts != maxAttempts || updated.TimeoutSecs != timeout || updated.Enabled != enabled {
		t.Fatalf("unexpected scalar fields: %+v", updated)
	}
	if string(updated.PayloadSchema) != string(schema) {
		t.Fatalf("unexpected payload schema: %s", string(updated.PayloadSchema))
	}
}

func TestHandleTriggerJob_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 120}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTriggerJob_GetJobError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleTriggerJob_IdempotencyLookupError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, errors.New("lookup failed")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`)
	r.Header.Set("Idempotency-Key", "idem-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleCancelRun_GetUpdatedRunError(t *testing.T) {
	t.Parallel()
	getRunCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return nil, errors.New("reload failed")
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-123", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleCancelRun_ListChildrenErrorStillSucceeds(t *testing.T) {
	t.Parallel()
	getRunCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCanceled}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, errors.New("list failed")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-123", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKComplete_StoreGetError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKComplete_UpdateError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("write failed")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKFail_StoreGetError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":"boom"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKFail_UpdateError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("write failed")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":"boom"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKComplete_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{"result":`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKFail_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTriggerJob_RunTTLSecs(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, RunTTLSecs: 600}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	expected := time.Now().Add(600 * time.Second)
	diff := capturedRun.ExpiresAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt diff = %v, want within 5s", diff)
	}
}

func TestHandleTriggerJob_DefaultTTL(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, RunTTLSecs: 0}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	expected := time.Now().Add(120 * time.Second)
	diff := capturedRun.ExpiresAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt diff = %v, want within 5s", diff)
	}
}

func TestHandleTriggerJob_ProjectQueuedQuotaExceeded(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			if projectID != "proj-1" {
				t.Fatalf("unexpected project id %q", projectID)
			}
			return &store.ProjectQuota{ProjectID: projectID, MaxQueuedRuns: 1}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 1, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected run not to be enqueued when project quota exceeded")
	}
}

func TestHandleTriggerJob_RateLimitExceeded(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, RateLimitMax: 2, RateLimitWindowSecs: 60}, nil
		},
		CountRunsForJobSinceFunc: func(_ context.Context, _ string, _ time.Time) (int, error) {
			return 2, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected run not to be enqueued when rate limit exceeded")
	}
}

func TestHandleTriggerJob_DedupWindowReturnsExistingRun(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, DedupWindowSecs: 300}, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, jobID string, payload json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			if jobID != "job-123" {
				t.Fatalf("unexpected job id %q", jobID)
			}
			if string(payload) != `{"a":1}` {
				t.Fatalf("unexpected canonical payload %s", string(payload))
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"a":1}}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected enqueue to be skipped when dedup window hit")
	}
}

type txAPIStoreMock struct {
	*APIStoreMock
}

func (m *txAPIStoreMock) WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error {
	return fn(ctx, fakeDBTX{})
}

type fakeDBTX struct{}

func (fakeDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (fakeDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("fake DBTX does not support Query")
}

func (fakeDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func TestHandleTriggerJob_DedupWindowRechecksInsideLimitGuard(t *testing.T) {
	t.Parallel()
	var findCalls int
	enqueued := false
	base := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, DedupWindowSecs: 300}, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			findCalls++
			if findCalls == 1 {
				return nil, nil
			}
			return &domain.JobRun{ID: "run-winner", Status: domain.StatusQueued}, nil
		},
	}
	base.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) {
		t.Fatal("dependencies should not be evaluated after guarded dedup hit")
		return true, nil
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, &txAPIStoreMock{APIStoreMock: base}, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"a":1}}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if findCalls != 2 {
		t.Fatalf("FindRecentRunByPayload calls = %d, want 2", findCalls)
	}
	if enqueued {
		t.Fatal("expected enqueue to be skipped when guarded dedup recheck hits")
	}
	if !strings.Contains(w.Body.String(), "run-winner") {
		t.Fatalf("response did not return dedup winner: %s", w.Body.String())
	}
}

func TestHandleTriggerJob_DelayedRunExpiresRelativeToScheduledAt(t *testing.T) {
	t.Parallel()
	scheduledAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	ttlSecs := 60
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		clone := *run
		capturedRun = &clone
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"scheduled_at":%q,"ttl_secs":%d}`, scheduledAt.Format(time.RFC3339), ttlSecs)
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil || capturedRun.ExpiresAt == nil {
		t.Fatal("expected enqueued run with expires_at")
	}
	want := scheduledAt.Add(time.Duration(ttlSecs) * time.Second)
	if capturedRun.ExpiresAt.Before(want.Add(-time.Second)) || capturedRun.ExpiresAt.After(want.Add(time.Second)) {
		t.Fatalf("expires_at = %s, want around %s", capturedRun.ExpiresAt.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestHandleTriggerJob_ExecutionWindowDelaysRun(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, ExecutionWindowCron: "0 0 1 1 *", Timezone: "UTC"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if capturedRun.Status != domain.StatusDelayed {
		t.Fatalf("expected delayed status, got %s", capturedRun.Status)
	}
	if capturedRun.ScheduledAt == nil {
		t.Fatal("expected scheduled_at to be set by execution window")
	}
	if !capturedRun.ScheduledAt.After(time.Now().Add(24 * time.Hour)) {
		t.Fatalf("expected execution window to push scheduled_at to future, got %v", capturedRun.ScheduledAt)
	}
}

func TestHandleCreateJob_WithRunTTL(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-ttl"
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"https://example.com","run_ttl_secs":300}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["run_ttl_secs"] != float64(300) {
		t.Fatalf("expected run_ttl_secs=300, got %v", resp["run_ttl_secs"])
	}
}

func TestHandleUpdateJob_WithRunTTL(t *testing.T) {
	t.Parallel()
	var updated *domain.Job
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old", EndpointURL: "https://example.com", Enabled: true, RunTTLSecs: 0}, nil
		},
		UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
			updated = job
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"run_ttl_secs":600}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updated == nil {
		t.Fatal("expected UpdateJob to be called")
	}
	if updated.RunTTLSecs != 600 {
		t.Fatalf("expected run_ttl_secs=600, got %d", updated.RunTTLSecs)
	}
}

func TestHealthReady_RedisDown(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(ctx context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 0, Executing: 0, Delayed: 0}, nil
		},
	}

	pinger := &mockPinger{err: fmt.Errorf("redis connection refused")}

	srv := newTestServerWithPinger(t, ms, &mockQueue{}, nil, pinger)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

func TestHealthReady_NoPinger(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(ctx context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 0, Executing: 0, Delayed: 0}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHealthReady_RedisOK(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(ctx context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 0, Executing: 0, Delayed: 0}, nil
		},
	}

	pinger := &mockPinger{err: nil}
	srv := newTestServerWithPinger(t, ms, &mockQueue{}, nil, pinger)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleListRunEvents_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	ms := &APIStoreMock{
		ListEventsByRunFilteredFunc: func(ctx context.Context, runID string, level, eventType string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
			if runID != "run-123" {
				t.Errorf("runID = %s, want run-123", runID)
			}
			return []domain.RunEvent{
				{ID: "evt-1", RunID: "run-123", Type: "log", Level: "info", Message: "started", CreatedAt: now},
				{ID: "evt-2", RunID: "run-123", Type: "log", Level: "error", Message: "failed", CreatedAt: now},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/runs/run-123/events", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var events []domain.RunEvent
	decodePaginatedList(t, w.Body.Bytes(), &events)
	if len(events) != 2 {
		t.Errorf("len(events) = %d, want 2", len(events))
	}
}

func TestHandleListRunEvents_WithLevelFilter(t *testing.T) {
	t.Parallel()
	var gotLevel string
	ms := &APIStoreMock{
		ListEventsByRunFilteredFunc: func(ctx context.Context, runID, level, eventType string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
			gotLevel = level
			return []domain.RunEvent{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/events?level=error", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLevel != "error" {
		t.Errorf("level = %q, want %q", gotLevel, "error")
	}
}

func TestHandleListRunEvents_WithTypeFilter(t *testing.T) {
	t.Parallel()
	var gotType string
	ms := &APIStoreMock{
		ListEventsByRunFilteredFunc: func(ctx context.Context, runID, level, eventType string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
			gotType = eventType
			return []domain.RunEvent{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/events?type=heartbeat", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotType != "heartbeat" {
		t.Errorf("type = %q, want %q", gotType, "heartbeat")
	}
}

func TestHandleListRunEvents_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListEventsByRunFilteredFunc: func(ctx context.Context, runID, level, eventType string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/events", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHandleListRunEvents_EmptyResult(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListEventsByRunFilteredFunc: func(ctx context.Context, runID, level, eventType string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
			return []domain.RunEvent{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/runs/run-1/events", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var events []domain.RunEvent
	decodePaginatedList(t, w.Body.Bytes(), &events)
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
}

func TestHandleListWebhookDeliveries_Success(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			return []domain.WebhookDelivery{
				{ID: "del-1", RunID: "run-1", JobID: "job-1", WebhookURL: "https://example.com/hook", Status: "delivered", Attempts: 1, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var deliveries []domain.WebhookDelivery
	decodePaginatedList(t, w.Body.Bytes(), &deliveries)
	if len(deliveries) != 1 {
		t.Errorf("len = %d, want 1", len(deliveries))
	}
}

func TestHandleListWebhookDeliveries_RedactsWebhookURLSecrets(t *testing.T) {
	t.Parallel()

	rawURL := "https://user:pass@hooks.example.com/services/T00/B00/token?secret=value#frag"
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(context.Context, string, string, int, *time.Time) ([]domain.WebhookDelivery, error) {
			return []domain.WebhookDelivery{{
				ID:         "del-1",
				WebhookURL: rawURL,
				Status:     domain.WebhookStatusFailed,
				CreatedAt:  time.Now().UTC(),
			}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "user:pass") || strings.Contains(w.Body.String(), "/services/") || strings.Contains(w.Body.String(), "secret=value") {
		t.Fatalf("response leaked sensitive webhook URL components: %s", w.Body.String())
	}
	var deliveries []domain.WebhookDelivery
	decodePaginatedList(t, w.Body.Bytes(), &deliveries)
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) = %d, want 1", len(deliveries))
	}
	if deliveries[0].WebhookURL != "https://hooks.example.com" {
		t.Fatalf("webhook_url = %q, want redacted host URL", deliveries[0].WebhookURL)
	}
}

func TestHandleListWebhookDeliveries_WithStatusFilter(t *testing.T) {
	t.Parallel()
	var gotStatus string
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			gotStatus = status
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries?status=pending", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotStatus != "pending" {
		t.Errorf("status = %q, want %q", gotStatus, "pending")
	}
}

func TestHandleListWebhookDeliveries_EnvironmentScopeFiltersForeignDeliveries(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(_ context.Context, projectID, status string, _ int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want proj-1", projectID)
			}
			if status != domain.WebhookStatusFailed {
				t.Fatalf("status = %q, want %q", status, domain.WebhookStatusFailed)
			}
			return []domain.WebhookDelivery{
				{ID: "del-staging", JobID: "job-staging", ProjectID: "proj-1", Status: domain.WebhookStatusFailed, CreatedAt: now.Add(-2 * time.Second)},
				{ID: "del-prod", JobID: "job-prod", ProjectID: "proj-1", Status: domain.WebhookStatusFailed, CreatedAt: now.Add(-1 * time.Second)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			switch id {
			case "job-prod":
				return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				t.Fatalf("unexpected job lookup %q", id)
				return nil, nil
			}
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	out, err := srv.handleListWebhookDeliveries(ctx, &ListWebhookDeliveriesInput{
		Status: domain.WebhookStatusFailed,
	})
	if err != nil {
		t.Fatalf("handleListWebhookDeliveries returned error: %v", err)
	}
	deliveries, ok := out.Body.Data.([]domain.WebhookDelivery)
	if !ok {
		t.Fatalf("response data type = %T, want []domain.WebhookDelivery", out.Body.Data)
	}
	if len(deliveries) != 1 || deliveries[0].ID != "del-prod" {
		t.Fatalf("deliveries = %#v, want only del-prod", deliveries)
	}
}

func TestHandleListWebhookDeliveries_WithLimit(t *testing.T) {
	t.Parallel()
	var gotLimit int
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			gotLimit = limit
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries?limit=10", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLimit != 11 { // handler passes limit+1 for has_more detection
		t.Errorf("limit = %d, want 11 (10+1)", gotLimit)
	}
}

func TestHandleListWebhookDeliveries_DefaultLimit(t *testing.T) {
	t.Parallel()
	var gotLimit int
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			gotLimit = limit
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLimit != 51 { // handler passes limit+1 (default 50+1)
		t.Errorf("limit = %d, want 51 (default+1)", gotLimit)
	}
}

func TestHandleListWebhookDeliveries_LimitCapped(t *testing.T) {
	t.Parallel()
	var gotLimit int
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			gotLimit = limit
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries?limit=200", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLimit != 101 { // handler passes limit+1 (capped 100+1)
		t.Errorf("limit = %d, want 101 (capped+1)", gotLimit)
	}
}

func TestHandleListWebhookDeliveries_InvalidLimit(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries?limit=abc", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleListWebhookDeliveries_NegativeLimit(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries?limit=-5", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleListWebhookDeliveries_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhook-deliveries", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHandleListWebhookDeliveries_NewRouteGroup(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListWebhookDeliveriesFunc: func(ctx context.Context, projectID, status string, limit int, _ *time.Time) ([]domain.WebhookDelivery, error) {
			if projectID != "proj-1" {
				t.Fatalf("project_id = %q, want proj-1", projectID)
			}
			return []domain.WebhookDelivery{{ID: "del-1", Status: domain.WebhookStatusPending, CreatedAt: time.Now().UTC()}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/webhooks/deliveries", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetWebhookDelivery_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
			if id != "del-1" {
				t.Fatalf("delivery id = %q, want del-1", id)
			}
			return &domain.WebhookDelivery{ID: id, Status: domain.WebhookStatusPending}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhooks/deliveries/del-1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetWebhookDelivery_RedactsWebhookURLSecrets(t *testing.T) {
	t.Parallel()

	rawURL := "https://user:pass@hooks.example.com/services/T00/B00/token?secret=value#frag"
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", WebhookURL: rawURL, Status: domain.WebhookStatusFailed}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedRequest(http.MethodGet, "/v1/webhooks/deliveries/del-1", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "user:pass") || strings.Contains(w.Body.String(), "/services/") || strings.Contains(w.Body.String(), "secret=value") {
		t.Fatalf("response leaked sensitive webhook URL components: %s", w.Body.String())
	}
	var delivery domain.WebhookDelivery
	if err := json.NewDecoder(w.Body).Decode(&delivery); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if delivery.WebhookURL != "https://hooks.example.com" {
		t.Fatalf("webhook_url = %q, want redacted host URL", delivery.WebhookURL)
	}
}

func TestHandleGetWebhookDelivery_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("webhook delivery not found")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhooks/deliveries/missing", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRetryWebhookDelivery_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: id, Status: domain.WebhookStatusDead}, nil
		},
		RetryWebhookDeliveryFunc: func(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: id, Status: domain.WebhookStatusPending, Attempts: 0}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/retry", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRetryWebhookDelivery_Conflict(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: id, Status: domain.WebhookStatusDelivered}, nil
		},
		RetryWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			t.Fatal("RetryWebhookDelivery should not be called")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/retry", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRetryWebhookDelivery_GetNotFoundErrorReturns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("webhook delivery not found")
		},
		RetryWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			t.Fatal("RetryWebhookDelivery should not be called when get returns not found")
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodPost, "/v1/webhooks/deliveries/missing/retry", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRetryWebhookDelivery_NoLongerRetriableReturns409(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", Status: domain.WebhookStatusFailed}, nil
		},
		RetryWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("webhook delivery not retriable")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/retry", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRetryWebhookDelivery(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		retryCalled := false
		ms := &APIStoreMock{
			GetWebhookDeliveryFunc: func(_ context.Context, id string) (*domain.WebhookDelivery, error) {
				if id != "del-1" {
					t.Fatalf("delivery id = %q, want del-1", id)
				}
				return &domain.WebhookDelivery{
					ID:        id,
					Status:    "failed",
					Attempts:  2,
					LastError: "boom",
				}, nil
			},
			RetryWebhookDeliveryFunc: func(_ context.Context, id string) (*domain.WebhookDelivery, error) {
				retryCalled = true
				return &domain.WebhookDelivery{
					ID:       id,
					Status:   "pending",
					Attempts: 0,
				}, nil
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhook-deliveries/del-1/retry", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		if !retryCalled {
			t.Fatal("expected RetryWebhookDelivery to be called")
		}

		var resp domain.WebhookDelivery
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Status != "pending" || resp.Attempts != 0 {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
				return nil, nil
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhook-deliveries/missing/retry", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("conflict when status is not failed", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
				return &domain.WebhookDelivery{ID: "del-1", Status: "delivered"}, nil
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhook-deliveries/del-1/retry", ""))

		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", w.Code)
		}
	})

	t.Run("get delivery store error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
				return nil, fmt.Errorf("db down")
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhook-deliveries/del-1/retry", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("retry delivery store error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
				return &domain.WebhookDelivery{ID: "del-1", Status: "failed"}, nil
			},
			RetryWebhookDeliveryFunc: func(_ context.Context, _ string) (*domain.WebhookDelivery, error) {
				return nil, fmt.Errorf("retry failed")
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhook-deliveries/del-1/retry", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestHandleTriggerJob_PriorityValidRange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		priority int
	}{
		{"zero_priority", 0},
		{"mid_priority", 5},
		{"max_priority", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
				if run.Priority != tt.priority {
					t.Errorf("priority = %d, want %d", run.Priority, tt.priority)
				}
				return nil
			}}
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, Enabled: true, TimeoutSecs: 30}, nil
				},
			}
			srv := newTestServer(t, ms, mq, nil)
			body := fmt.Sprintf(`{"payload":{},"priority":%d}`, tt.priority)
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				t.Errorf("status = %d, want 201; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleRetryWebhookDelivery_RedactsWebhookURLSecrets(t *testing.T) {
	t.Parallel()

	rawURL := "https://user:pass@hooks.example.com/services/T00/B00/token?secret=value#frag"
	ms := &APIStoreMock{
		GetWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", WebhookURL: rawURL, Status: domain.WebhookStatusFailed}, nil
		},
		RetryWebhookDeliveryFunc: func(context.Context, string) (*domain.WebhookDelivery, error) {
			return &domain.WebhookDelivery{ID: "del-1", WebhookURL: rawURL, Status: domain.WebhookStatusPending}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedRequest(http.MethodPost, "/v1/webhooks/deliveries/del-1/retry", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "user:pass") || strings.Contains(w.Body.String(), "/services/") || strings.Contains(w.Body.String(), "secret=value") {
		t.Fatalf("response leaked sensitive webhook URL components: %s", w.Body.String())
	}
	var delivery domain.WebhookDelivery
	if err := json.NewDecoder(w.Body).Decode(&delivery); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if delivery.WebhookURL != "https://hooks.example.com" {
		t.Fatalf("webhook_url = %q, want redacted host URL", delivery.WebhookURL)
	}
}

func TestHandleTriggerJob_PriorityTooHigh(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Enabled: true, TimeoutSecs: 30}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{},"priority":11}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Priority") || !strings.Contains(w.Body.String(), "max") {
		t.Errorf("body = %s, want priority error message", w.Body.String())
	}
}

func TestHandleTriggerJob_PriorityNegative(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Enabled: true, TimeoutSecs: 30}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{},"priority":-1}`)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
}

func TestHandleTriggerJob_PriorityBoundary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		priority   int
		wantStatus int
	}{
		{"negative_one", -1, http.StatusUnprocessableEntity},
		{"zero", 0, http.StatusCreated},
		{"ten", 10, http.StatusCreated},
		{"eleven", 11, http.StatusUnprocessableEntity},
		{"large_negative", -100, http.StatusUnprocessableEntity},
		{"large_positive", 999, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, Enabled: true, TimeoutSecs: 30}, nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}, nil)
			body := fmt.Sprintf(`{"payload":{},"priority":%d}`, tt.priority)
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("[priority=%d] status = %d, want %d; body: %s", tt.priority, w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestValidateWorkflowConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		cronExpr         string
		cronTimezone     string
		maxParallelSteps int
		wantErr          bool
		wantErrContains  string
	}{
		{
			name:             "valid_no_cron",
			cronExpr:         "",
			cronTimezone:     "",
			maxParallelSteps: 0,
		},
		{
			name:             "valid_with_cron",
			cronExpr:         "*/5 * * * *",
			cronTimezone:     "",
			maxParallelSteps: 0,
		},
		{
			name:             "valid_with_cron_and_timezone",
			cronExpr:         "0 9 * * 1-5",
			cronTimezone:     "America/New_York",
			maxParallelSteps: 2,
		},
		{
			name:             "negative_max_parallel_steps",
			cronExpr:         "",
			cronTimezone:     "",
			maxParallelSteps: -1,
			wantErr:          true,
			wantErrContains:  "max_parallel_steps must be >= 0",
		},
		{
			name:             "invalid_cron_expression",
			cronExpr:         "not-a-cron",
			cronTimezone:     "",
			maxParallelSteps: 0,
			wantErr:          true,
			wantErrContains:  "invalid cron expression",
		},
		{
			name:             "invalid_cron_timezone",
			cronExpr:         "*/5 * * * *",
			cronTimezone:     "Invalid/Timezone",
			maxParallelSteps: 0,
			wantErr:          true,
			wantErrContains:  "invalid cron_timezone",
		},
		{
			name:             "valid_cron_timezone_empty_with_cron",
			cronExpr:         "0 0 * * *",
			cronTimezone:     "",
			maxParallelSteps: 0,
		},
		{
			name:             "zero_max_parallel_steps_valid",
			cronExpr:         "",
			cronTimezone:     "",
			maxParallelSteps: 0,
		},
		{
			name:             "positive_max_parallel_steps_valid",
			cronExpr:         "",
			cronTimezone:     "",
			maxParallelSteps: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateWorkflowConfig(tt.cronExpr, tt.cronTimezone, tt.maxParallelSteps)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHandleTriggerJob_DailyCostBudgetExceeded(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxDailyCostMicrousd: 5000, Timezone: "UTC"}, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, _ string) (int64, error) {
			return 5000, nil // at limit
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected run not to be enqueued when daily cost budget exceeded")
	}
}

func TestHandleTriggerJob_DailyCostBudgetOK(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxDailyCostMicrousd: 5000, Timezone: "UTC"}, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, _ string) (int64, error) {
			return 3000, nil // under limit
		},
		CreateRunFunc: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !enqueued {
		t.Fatal("expected run to be enqueued when daily cost budget not exceeded")
	}
}

func TestHandleCreateJob_InvalidRetryStrategy(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "Test Job",
		"slug": "test-job",
		"endpoint_url": "https://example.com/webhook",
		"retry_strategy": "banana"
	}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid retry_strategy, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "RetryStrategy") || !strings.Contains(w.Body.String(), "oneof") {
		t.Fatalf("expected error about retry_strategy, got: %s", w.Body.String())
	}
}

func TestHandleCreateJob_NegativeRetryDelays(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "Test Job",
		"slug": "test-job",
		"endpoint_url": "https://example.com/webhook",
		"retry_strategy": "custom",
		"retry_delays_secs": [-5, 10, 30]
	}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative retry_delays_secs, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "retry_delays_secs values must be positive") {
		t.Fatalf("expected error about positive values, got: %s", w.Body.String())
	}
}

func TestHandleCreateJob_ValidRetryStrategy(t *testing.T) {
	t.Parallel()
	strategies := []string{"exponential", "linear", "fixed", "custom"}
	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				CreateJobFunc: func(_ context.Context, _ *domain.Job) error { return nil },
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)

			body := fmt.Sprintf(`{
				"project_id": "proj-1",
				"name": "Test Job",
				"slug": "test-job-%s",
				"endpoint_url": "https://example.com/webhook",
				"retry_strategy": "%s"
			}`, strategy, strategy)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", body))

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201 for valid strategy %q, got %d: %s", strategy, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleUpdateJob_InvalidRetryStrategy(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"retry_strategy": "banana"}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid retry_strategy on update, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateJob_NegativeRetryDelays(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Test", Slug: "test", EndpointURL: "https://example.com", Enabled: true}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123", `{"retry_delays_secs": [0, -1, 5]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative retry_delays_secs on update, got %d: %s", w.Code, w.Body.String())
	}
}
