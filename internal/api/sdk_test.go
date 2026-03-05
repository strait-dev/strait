package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

func generateRunToken(t *testing.T, runID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   runID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	signed, err := token.SignedString([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func sdkRequest(t *testing.T, method, path, runID, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+generateRunToken(t, runID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", runID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return r
}

func generateExpiredRunToken(t *testing.T, runID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   runID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
	})
	signed, err := token.SignedString([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestHandleSDKLog_Success(t *testing.T) {
	var insertCalled atomic.Bool
	ms := &mockAPIStore{
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			insertCalled.Store(true)
			if event.RunID != "run-123" {
				t.Fatalf("expected run id run-123, got %s", event.RunID)
			}
			if event.Type != domain.EventError {
				t.Fatalf("expected type error, got %s", event.Type)
			}
			if event.Level != "warn" {
				t.Fatalf("expected level warn, got %s", event.Level)
			}
			if event.Message != "something happened" {
				t.Fatalf("expected message, got %s", event.Message)
			}
			if string(event.Data) != `{"code":123}` {
				t.Fatalf("expected data payload, got %s", string(event.Data))
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"type":"error","level":"warn","message":"something happened","data":{"code":123}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !insertCalled.Load() {
		t.Fatal("expected InsertEvent to be called")
	}
}

func TestHandleSDKLog_MissingMessage(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"type":"log"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKLog_DefaultsEventType(t *testing.T) {
	var insertCalled atomic.Bool
	ms := &mockAPIStore{
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			insertCalled.Store(true)
			if event.Type != domain.EventLog {
				t.Fatalf("expected default event type log, got %s", event.Type)
			}
			if event.Level != "info" {
				t.Fatalf("expected default level info, got %s", event.Level)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"message":"hello"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !insertCalled.Load() {
		t.Fatal("expected InsertEvent to be called")
	}
}

func TestHandleSDKLog_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		insertEventFn: func(_ context.Context, _ *domain.RunEvent) error {
			return errors.New("boom")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"message":"hello"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKHeartbeat_Success(t *testing.T) {
	var updateCalled atomic.Bool
	ms := &mockAPIStore{
		updateHeartbeatFn: func(_ context.Context, id string) error {
			updateCalled.Store(true)
			if id != "run-123" {
				t.Fatalf("expected run id run-123, got %s", id)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", "run-123", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !updateCalled.Load() {
		t.Fatal("expected UpdateHeartbeat to be called")
	}
}

func TestHandleSDKHeartbeat_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		updateHeartbeatFn: func(_ context.Context, _ string) error {
			return errors.New("boom")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", "run-123", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKComplete_Success(t *testing.T) {
	getRunCalls := 0
	var updateCalled atomic.Bool
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
			updateCalled.Store(true)
			if id != "run-123" {
				t.Fatalf("expected run id run-123, got %s", id)
			}
			if from != domain.StatusExecuting || to != domain.StatusCompleted {
				t.Fatalf("unexpected transition %s -> %s", from, to)
			}
			if _, ok := fields["finished_at"]; !ok {
				t.Fatal("expected finished_at field")
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !updateCalled.Load() {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if getRunCalls != 2 {
		t.Fatalf("expected GetRun to be called twice, got %d", getRunCalls)
	}
}

func TestHandleSDKComplete_WithResult(t *testing.T) {
	getRunCalls := 0
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, Result: json.RawMessage(`{"ok":true}`)}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, fields map[string]any) error {
			result, ok := fields["result"].(json.RawMessage)
			if !ok {
				t.Fatalf("expected result field to be json.RawMessage, got %T", fields["result"])
			}
			if string(result) != `{"ok":true}` {
				t.Fatalf("expected result payload, got %s", string(result))
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{"result":{"ok":true}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKComplete_RunNotFound(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleSDKComplete_Conflict(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunConflict
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandleSDKFail_Success(t *testing.T) {
	getRunCalls := 0
	var updateCalled atomic.Bool
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusFailed, Error: "boom"}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, fields map[string]any) error {
			updateCalled.Store(true)
			if from != domain.StatusExecuting || to != domain.StatusFailed {
				t.Fatalf("unexpected transition %s -> %s", from, to)
			}
			if fields["error"] != "boom" {
				t.Fatalf("expected error field boom, got %v", fields["error"])
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":"boom"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !updateCalled.Load() {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if getRunCalls != 2 {
		t.Fatalf("expected GetRun to be called twice, got %d", getRunCalls)
	}
}

func TestHandleSDKFail_MissingError(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKFail_RunNotFound(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":"boom"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleSDKFail_Conflict(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunConflict
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":"boom"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandleSDKSpawn_Success(t *testing.T) {
	var getJobCalled atomic.Bool
	var enqueueCalled atomic.Bool
	ms := &mockAPIStore{
		getJobBySlugFn: func(_ context.Context, projectID, slug string) (*domain.Job, error) {
			getJobCalled.Store(true)
			if projectID != "proj-1" || slug != "child-job" {
				t.Fatalf("unexpected project/slug %s/%s", projectID, slug)
			}
			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: slug}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueueCalled.Store(true)
			if run.JobID != "job-123" {
				t.Fatalf("expected job id job-123, got %s", run.JobID)
			}
			if run.TriggeredBy != domain.TriggerSpawn {
				t.Fatalf("expected triggered_by spawn, got %s", run.TriggeredBy)
			}
			if run.ParentRunID != "run-parent" {
				t.Fatalf("expected parent run id run-parent, got %s", run.ParentRunID)
			}
			if string(run.Payload) != `{"x":1}` {
				t.Fatalf("expected payload, got %s", string(run.Payload))
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"job_slug":"child-job","project_id":"proj-1","payload":{"x":1}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !getJobCalled.Load() {
		t.Fatal("expected GetJobBySlug to be called")
	}
	if !enqueueCalled.Load() {
		t.Fatal("expected Enqueue to be called")
	}
}

func TestHandleSDKSpawn_MissingFields(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKSpawn_JobNotFound(t *testing.T) {
	ms := &mockAPIStore{
		getJobBySlugFn: func(_ context.Context, _, _ string) (*domain.Job, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"job_slug":"child-job","project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleSDKSpawn_EnqueueError(t *testing.T) {
	ms := &mockAPIStore{
		getJobBySlugFn: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: projectID}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue down")
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"job_slug":"child-job","project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSDKAuth_MissingToken(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSDKAuth_InvalidToken(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer not-a-jwt")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSDKAuth_TokenRunIDMismatch(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+generateRunToken(t, "run-999"))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", "run-123")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSDKAuth_ExpiredToken(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+generateExpiredRunToken(t, "run-123"))

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleHealthReady_Success(t *testing.T) {
	ms := &mockAPIStore{
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleHealthReady_StoreError(t *testing.T) {
	ms := &mockAPIStore{
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return nil, errors.New("db unavailable")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleGetRun_Success(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/run-123", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetRun_NotFound(t *testing.T) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/run-123", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListRuns_Success(t *testing.T) {
	var listCalled atomic.Bool
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			listCalled.Store(true)
			if projectID != "proj-1" {
				t.Fatalf("expected project_id proj-1, got %s", projectID)
			}
			if status == nil || *status != domain.StatusExecuting {
				t.Fatalf("expected status executing, got %v", status)
			}
			if limit != 100 {
				t.Fatalf("expected limit to be clamped to 100, got %d", limit)
			}
			if cursor == nil {
				t.Fatal("expected cursor to be parsed")
			}
			return []domain.JobRun{{ID: "run-1", ProjectID: projectID, Status: domain.StatusExecuting}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	cursor := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/?project_id=proj-1&status=executing&limit=500&cursor="+cursor, "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !listCalled.Load() {
		t.Fatal("expected ListRunsByProject to be called")
	}
}

func TestHandleListRuns_MissingProjectID(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_InvalidLimit(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/?project_id=proj-1&limit=abc", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_InvalidCursor(t *testing.T) {
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/?project_id=proj-1&cursor=not-a-time", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_InvalidStatus(t *testing.T) {
	called := false
	ms := &mockAPIStore{
		listRunsByProjectFn: func(_ context.Context, _ string, _ *domain.RunStatus, _ int, _ *time.Time) ([]domain.JobRun, error) {
			called = true
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/?project_id=proj-1&status=definitely-not-valid", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if called {
		t.Fatal("expected ListRunsByProject to not be called for invalid status")
	}
}

func TestHandleListChildRuns_Success(t *testing.T) {
	ms := &mockAPIStore{
		listChildRunsFn: func(_ context.Context, parentRunID string) ([]domain.JobRun, error) {
			if parentRunID != "run-parent" {
				t.Fatalf("expected parent run id run-parent, got %s", parentRunID)
			}
			return []domain.JobRun{{ID: "run-child", ParentRunID: parentRunID}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/run-parent/children", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
