package api

import (
	"context"
	"encoding/json"
	"errors"
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
