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

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
)

func TestHandleUpdateJob_Success(t *testing.T) {
	var updated *domain.Job
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old Name", Slug: "job", EndpointURL: "https://example.com", Enabled: true}, nil
		},
		updateJobFn: func(_ context.Context, job *domain.Job) error {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old", EndpointURL: "https://example.com"}, nil
		},
		updateJobFn: func(_ context.Context, _ *domain.Job) error {
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
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"","name":"Job","slug":"job","endpoint_url":"https://example.com"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJob_InvalidURL(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"not-a-url"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJob_InvalidCron(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	body := `{"project_id":"proj-1","name":"Job","slug":"job","endpoint_url":"https://example.com","cron":"bad cron"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateJob_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, _ *domain.Job) error {
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
	var got *domain.Job
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, job *domain.Job) error {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-404", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteJob_StoreGetError(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-500", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeleteJob_StoreUpdateError(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, EndpointURL: "https://example.com", Enabled: true}, nil
		},
		updateJobFn: func(_ context.Context, _ *domain.Job) error {
			return errors.New("update failed")
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
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
	getRunCalls := 0
	updates := make(map[string]domain.RunStatus)

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCanceled}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _ domain.RunStatus, to domain.RunStatus, _ map[string]any) error {
			updates[id] = to
			return nil
		},
		listChildRunsFn: func(_ context.Context, parentRunID string) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "child-running", ParentRunID: parentRunID, Status: domain.StatusQueued},
				{ID: "child-done", ParentRunID: parentRunID, Status: domain.StatusCompleted},
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
	if updates["child-running"] != domain.StatusCanceled {
		t.Fatalf("expected running child to be canceled, got %q", updates["child-running"])
	}
	if _, ok := updates["child-done"]; ok {
		t.Fatal("expected terminal child to not be updated")
	}
}

func TestHandleTriggerJob_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
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
	enqueued := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if jobID != "job-123" || key != "same-key" {
				t.Fatalf("unexpected idempotency lookup args: %s %s", jobID, key)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
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
	var enqueued *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 120}, nil
		},
	}
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	srv.config.FFPayloadValidation = true

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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	srv.config.FFPayloadValidation = true

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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 120}, nil
		},
	}
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

func TestValidateURL_ValidHTTPS(t *testing.T) {
	if err := validateURL("https://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateURL_ValidHTTP(t *testing.T) {
	if err := validateURL("http://example.com"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateURL_InvalidScheme(t *testing.T) {
	if err := validateURL("ftp://example.com"); err == nil {
		t.Fatal("expected error for invalid scheme")
	}
}

func TestValidateURL_NoHost(t *testing.T) {
	if err := validateURL("http://"); err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestValidateURL_LoopbackIP(t *testing.T) {
	if err := validateURL("http://127.0.0.1"); err == nil {
		t.Fatal("expected error for loopback IP")
	}
}

func TestValidateURL_PrivateIP(t *testing.T) {
	if err := validateURL("http://192.168.1.1"); err == nil {
		t.Fatal("expected error for private IP")
	}
}

func TestValidateURL_InvalidURL(t *testing.T) {
	if err := validateURL("://bad"); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestHandleStats_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
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
	ms := &mockAPIStore{
		listChildRunsFn: func(_ context.Context, _ string) ([]domain.JobRun, error) {
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
	ms := &mockAPIStore{
		listChildRunsFn: func(_ context.Context, _ string) ([]domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		listJobsFn: func(_ context.Context, _ string) ([]domain.Job, error) {
			return nil, errors.New("db down")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/jobs/?project_id=proj-1", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetRun_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
		updateJobFn: func(_ context.Context, job *domain.Job) error {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
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
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
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
	getRunCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return nil, errors.New("reload failed")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string) ([]domain.JobRun, error) {
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
	getRunCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCanceled}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string) ([]domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
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
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{"result":`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKFail_InvalidBody(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTriggerJob_RunTTLSecs(t *testing.T) {
	var capturedRun *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, RunTTLSecs: 600}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
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
	var capturedRun *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, RunTTLSecs: 0}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
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

func TestHandleCreateJob_WithRunTTL(t *testing.T) {
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, job *domain.Job) error {
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
	var updated *domain.Job
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, Name: "Old", EndpointURL: "https://example.com", Enabled: true, RunTTLSecs: 0}, nil
		},
		updateJobFn: func(_ context.Context, job *domain.Job) error {
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
	ms := &mockAPIStore{
		queueStatsFn: func(ctx context.Context) (*store.QueueStats, error) {
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
	ms := &mockAPIStore{
		queueStatsFn: func(ctx context.Context) (*store.QueueStats, error) {
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
	ms := &mockAPIStore{
		queueStatsFn: func(ctx context.Context) (*store.QueueStats, error) {
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
	now := time.Now().UTC().Truncate(time.Second)
	ms := &mockAPIStore{
		listEventsByRunFilteredFn: func(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error) {
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
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("len(events) = %d, want 2", len(events))
	}
}

func TestHandleListRunEvents_WithLevelFilter(t *testing.T) {
	var gotLevel string
	ms := &mockAPIStore{
		listEventsByRunFilteredFn: func(ctx context.Context, runID, level, eventType string) ([]domain.RunEvent, error) {
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
	var gotType string
	ms := &mockAPIStore{
		listEventsByRunFilteredFn: func(ctx context.Context, runID, level, eventType string) ([]domain.RunEvent, error) {
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
	ms := &mockAPIStore{
		listEventsByRunFilteredFn: func(ctx context.Context, runID, level, eventType string) ([]domain.RunEvent, error) {
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
	ms := &mockAPIStore{
		listEventsByRunFilteredFn: func(ctx context.Context, runID, level, eventType string) ([]domain.RunEvent, error) {
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
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
}

func TestHandleListWebhookDeliveries_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ms := &mockAPIStore{
		listWebhookDeliveriesFn: func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
			return []domain.WebhookDelivery{
				{ID: "del-1", RunID: "run-1", JobID: "job-1", WebhookURL: "https://example.com/hook", Status: "delivered", Attempts: 1, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var deliveries []domain.WebhookDelivery
	if err := json.Unmarshal(w.Body.Bytes(), &deliveries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(deliveries) != 1 {
		t.Errorf("len = %d, want 1", len(deliveries))
	}
}

func TestHandleListWebhookDeliveries_WithStatusFilter(t *testing.T) {
	var gotStatus string
	ms := &mockAPIStore{
		listWebhookDeliveriesFn: func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
			gotStatus = status
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries?status=pending", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotStatus != "pending" {
		t.Errorf("status = %q, want %q", gotStatus, "pending")
	}
}

func TestHandleListWebhookDeliveries_WithLimit(t *testing.T) {
	var gotLimit int
	ms := &mockAPIStore{
		listWebhookDeliveriesFn: func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
			gotLimit = limit
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries?limit=10", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLimit != 10 {
		t.Errorf("limit = %d, want 10", gotLimit)
	}
}

func TestHandleListWebhookDeliveries_DefaultLimit(t *testing.T) {
	var gotLimit int
	ms := &mockAPIStore{
		listWebhookDeliveriesFn: func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
			gotLimit = limit
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLimit != 50 {
		t.Errorf("limit = %d, want 50 (default)", gotLimit)
	}
}

func TestHandleListWebhookDeliveries_LimitCapped(t *testing.T) {
	var gotLimit int
	ms := &mockAPIStore{
		listWebhookDeliveriesFn: func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
			gotLimit = limit
			return []domain.WebhookDelivery{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries?limit=200", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLimit != 100 {
		t.Errorf("limit = %d, want 100 (capped)", gotLimit)
	}
}

func TestHandleListWebhookDeliveries_InvalidLimit(t *testing.T) {
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries?limit=abc", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleListWebhookDeliveries_NegativeLimit(t *testing.T) {
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries?limit=-5", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleListWebhookDeliveries_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		listWebhookDeliveriesFn: func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedRequest(http.MethodGet, "/v1/webhook-deliveries", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
