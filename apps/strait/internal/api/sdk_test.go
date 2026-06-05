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

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

func generateRunToken(t *testing.T, runID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, runTokenClaims{
		Attempt: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait:run-token",
			Subject:   runID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(testJWTSigningKey))
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
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, runTokenClaims{
		Attempt: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "strait:run-token",
			Subject:   runID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	})
	signed, err := token.SignedString([]byte(testJWTSigningKey))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestHandleSDKLog_Success(t *testing.T) {
	t.Parallel()
	var insertCalled atomic.Bool
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
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
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"type":"log"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleSDKLog_DefaultsEventType(t *testing.T) {
	t.Parallel()
	var insertCalled atomic.Bool
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
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
	t.Parallel()
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, _ *domain.RunEvent) error {
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

func TestHandleSDKProgress_Success(t *testing.T) {
	t.Parallel()
	var insertCalled atomic.Bool
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			insertCalled.Store(true)
			if event.Type != domain.EventProgress {
				t.Fatalf("event type = %s, want %s", event.Type, domain.EventProgress)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/progress", "run-123", `{"percent":45,"message":"working","step":"phase-1","eta_seconds":30}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !insertCalled.Load() {
		t.Fatal("expected InsertEvent to be called")
	}
}

func TestHandleSDKProgress_InvalidPercent(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/progress", "run-123", `{"percent":101,"message":"working"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleSDKAnnotate_Success(t *testing.T) {
	t.Parallel()
	updated := false
	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, id string, annotations map[string]string) error {
			updated = true
			if id != "run-123" {
				t.Fatalf("run id = %q, want run-123", id)
			}
			if annotations["env"] != "prod" || annotations["region"] != "eu" {
				t.Fatalf("annotations = %+v", annotations)
			}
			return nil
		},
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Metadata: map[string]string{"env": "prod", "region": "eu"}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", `{"annotations":{"env":"prod","region":"eu"}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !updated {
		t.Fatal("expected UpdateRunMetadata to be called")
	}
}

func TestHandleSDKAnnotate_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			return store.ErrRunNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", `{"annotations":{"env":"prod"}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleSDKAnnotate_InvalidPayload(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", `{"annotations":{}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleSDKAnnotate_TooManyAnnotations(t *testing.T) {
	t.Parallel()
	annotations := make(map[string]string)
	for i := range 51 {
		annotations[strings.Repeat("k", i+1)] = "v"
	}

	payload, err := json.Marshal(map[string]any{"annotations": annotations})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			t.Fatal("UpdateRunMetadata should not be called for invalid annotations")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", string(payload))

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too many annotations (max 50)") {
		t.Fatalf("expected too-many-annotations error, got %s", w.Body.String())
	}
}

func TestHandleSDKAnnotate_AnnotationKeyTooLong(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(map[string]any{
		"annotations": map[string]string{
			strings.Repeat("k", 129): "prod",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			t.Fatal("UpdateRunMetadata should not be called for invalid annotations")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", string(payload))

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "annotation key too long (max 128 characters)") {
		t.Fatalf("expected key-too-long error, got %s", w.Body.String())
	}
}

func TestHandleSDKAnnotate_AnnotationValueTooLong(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(map[string]any{
		"annotations": map[string]string{
			"env": strings.Repeat("v", 1025),
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			t.Fatal("UpdateRunMetadata should not be called for invalid annotations")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", string(payload))

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "annotation value too long (max 1024 characters)") {
		t.Fatalf("expected value-too-long error, got %s", w.Body.String())
	}
}

func TestHandleSDKCheckpoint_Success(t *testing.T) {
	t.Parallel()
	created := false
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		CreateRunCheckpointFunc: func(_ context.Context, checkpoint *domain.RunCheckpoint) error {
			created = true
			if checkpoint.RunID != "run-123" {
				t.Fatalf("run_id = %q, want run-123", checkpoint.RunID)
			}
			if len(checkpoint.State) == 0 {
				t.Fatal("expected non-empty checkpoint state")
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/checkpoint", "run-123", `{"state":{"cursor":12}}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created {
		t.Fatal("expected CreateRunCheckpoint to be called")
	}
}

func TestSDKUsageRoute_NotRegistered(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/usage", "run-123", `{"usage_units":1}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for launch-inactive usage route, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKOutput_SchemaValidation(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/output", "run-123", `{"output_key":"final","schema":{"type":"object","required":["name"]},"value":{"age":12}}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKHeartbeat_Success(t *testing.T) {
	t.Parallel()
	var updateCalled atomic.Bool
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(_ context.Context, id string) error {
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
	t.Parallel()
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(_ context.Context, _ string) error {
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

func TestSDKRunToken_RevalidatesAfterDecodeBeforeMutation(t *testing.T) {
	t.Parallel()
	var statusCalls atomic.Int32
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(context.Context, string) error {
			t.Fatal("heartbeat mutation must not run after post-decode terminal revalidation")
			return nil
		},
	}
	ms.SetRunTokenStateFunc(func(context.Context, string) (domain.RunStatus, int, string, error) {
		if statusCalls.Add(1) == 1 {
			return domain.StatusExecuting, 1, "proj-1", nil
		}
		return domain.StatusCompleted, 1, "proj-1", nil
	})
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", "run-123", "")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", w.Code, w.Body.String())
	}
	if statusCalls.Load() != 2 {
		t.Fatalf("expected pre-auth and post-decode status checks, got %d", statusCalls.Load())
	}
}

func TestHandleSDKComplete_Success(t *testing.T) {
	t.Parallel()
	getRunCalls := 0
	var updateCalled atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
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
	t.Parallel()
	getRunCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusCompleted, Result: json.RawMessage(`{"ok":true}`)}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, fields map[string]any) error {
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
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
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
	t.Parallel()
	getRunCalls := 0
	var updateCalled atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			getRunCalls++
			if getRunCalls == 1 {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			}
			return &domain.JobRun{ID: id, Status: domain.StatusFailed, Error: "boom"}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from, to domain.RunStatus, fields map[string]any) error {
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
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleSDKFail_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
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
	t.Parallel()
	var getJobCalled atomic.Bool
	var enqueueCalled atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, slug string) (*domain.Job, error) {
			getJobCalled.Store(true)
			if projectID != "proj-1" || slug != "child-job" {
				t.Fatalf("unexpected project/slug %s/%s", projectID, slug)
			}
			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: slug}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
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
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleSDKSpawn_JobNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobBySlugFunc: func(_ context.Context, _, _ string) (*domain.Job, error) {
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
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		GetJobBySlugFunc: func(_ context.Context, projectID, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-123", ProjectID: projectID}, nil
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
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"job_slug":"child-job","project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSDKComplete_ResumesParentWhenDescendantsTerminal(t *testing.T) {
	t.Parallel()
	getRunCalls := 0
	updatedParent := false
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id == "run-child" {
				getRunCalls++
				if getRunCalls == 1 {
					return &domain.JobRun{ID: id, Status: domain.StatusExecuting, ParentRunID: "run-parent"}, nil
				}
				return &domain.JobRun{ID: id, Status: domain.StatusCompleted, ParentRunID: "run-parent"}, nil
			}
			if id == "run-parent" {
				return &domain.JobRun{ID: id, Status: domain.StatusWaiting}, nil
			}
			return nil, store.ErrRunNotFound
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, from, to domain.RunStatus, _ map[string]any) error {
			if id == "run-parent" {
				if from != domain.StatusWaiting || to != domain.StatusQueued {
					t.Fatalf("unexpected parent transition %s -> %s", from, to)
				}
				updatedParent = true
				return nil
			}
			if id == "run-child" && to == domain.StatusCompleted {
				return nil
			}
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, parentRunID string) (bool, error) {
			if parentRunID != "run-parent" {
				t.Fatalf("parent_run_id = %q, want run-parent", parentRunID)
			}
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-child/complete", "run-child", `{}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !updatedParent {
		t.Fatal("expected parent run to be resumed")
	}
}

func TestSDKAuth_MissingToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSDKAuth_InvalidToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

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
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

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
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+generateExpiredRunToken(t, "run-123"))

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSDKAuth_SDKVersionHeaders_Modern(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", "run-123", "")
	r.Header.Set("X-SDK-Version", "2.1.0")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-SDK-Version-Accepted"); got != "2.1.0" {
		t.Fatalf("X-SDK-Version-Accepted = %q, want %q", got, "2.1.0")
	}
	if got := w.Header().Get("X-SDK-Capabilities"); got != "progress,checkpoint" {
		t.Fatalf("X-SDK-Capabilities = %q, want %q", got, "progress,checkpoint")
	}
}

func TestSDKAuth_SDKVersionHeaders_Legacy(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", "run-123", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-SDK-Version-Accepted"); got != "legacy" {
		t.Fatalf("X-SDK-Version-Accepted = %q, want %q", got, "legacy")
	}
	if got := w.Header().Get("X-SDK-Capabilities"); got != "none" {
		t.Fatalf("X-SDK-Capabilities = %q, want %q", got, "none")
	}
}

func TestHandleHealthReady_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
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
	t.Parallel()
	ms := &APIStoreMock{
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
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
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
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
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
	t.Parallel()
	var listCalled atomic.Bool
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, _ *domain.ExecutionMode, _ *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			listCalled.Store(true)
			if projectID != "proj-1" {
				t.Fatalf("expected project_id proj-1, got %s", projectID)
			}
			if metadataKey != nil || metadataValue != nil {
				t.Fatalf("expected metadata filters to be nil, got key=%v value=%v", metadataKey, metadataValue)
			}
			if status == nil || *status != domain.StatusExecuting {
				t.Fatalf("expected status executing, got %v", status)
			}
			if limit != 101 { // handler passes limit+1 for has_more detection
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
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?status=executing&limit=500&cursor="+cursor, "", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !listCalled.Load() {
		t.Fatal("expected ListRunsByProject to be called")
	}
}

func TestHandleListRuns_MetadataFilter(t *testing.T) {
	t.Parallel()
	var listCalled atomic.Bool
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, _ *domain.ExecutionMode, _ *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			listCalled.Store(true)
			if projectID != "proj-1" {
				t.Fatalf("expected project_id proj-1, got %s", projectID)
			}
			if status != nil {
				t.Fatalf("expected status nil, got %v", *status)
			}
			if metadataKey == nil || *metadataKey != "env" {
				t.Fatalf("expected metadata_key env, got %v", metadataKey)
			}
			if metadataValue == nil || *metadataValue != "prod" {
				t.Fatalf("expected metadata_value prod, got %v", metadataValue)
			}
			if limit != 51 { // handler passes limit+1 (default 50 + 1)
				t.Fatalf("expected default limit 50, got %d", limit)
			}
			if cursor != nil {
				t.Fatalf("expected nil cursor, got %v", cursor)
			}
			return []domain.JobRun{{ID: "run-1", ProjectID: projectID}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?metadata_key=env&metadata_value=prod", "", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !listCalled.Load() {
		t.Fatal("expected ListRunsByProject to be called")
	}
}

func TestHandleListRuns_MetadataValueWithoutKey(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?metadata_value=prod", "", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/", "")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?limit=abc", "", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_InvalidCursor(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?cursor=not-a-time", "", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRuns_InvalidStatus(t *testing.T) {
	t.Parallel()
	called := false
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			called = true
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?status=definitely-not-valid", "", "proj-1")

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if called {
		t.Fatal("expected ListRunsByProject to not be called for invalid status")
	}
}

func TestHandleListChildRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListChildRunsFunc: func(_ context.Context, parentRunID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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

func TestHandleSDKContinue_Success(t *testing.T) {
	t.Parallel()
	var enqueuedRun *domain.JobRun
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:           id,
				JobID:        "job-1",
				ProjectID:    "proj-1",
				Status:       domain.StatusExecuting,
				LineageDepth: 2,
				Priority:     5,
				Payload:      json.RawMessage(`{"original":true}`),
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/continue", "run-parent", `{"payload":{"step":2}}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueuedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueuedRun.ContinuationOf != "run-parent" {
		t.Fatalf("expected continuation_of=run-parent, got %s", enqueuedRun.ContinuationOf)
	}
	if enqueuedRun.LineageDepth != 3 {
		t.Fatalf("expected lineage_depth=3, got %d", enqueuedRun.LineageDepth)
	}
	if enqueuedRun.Priority != 5 {
		t.Fatalf("expected priority=5, got %d", enqueuedRun.Priority)
	}
	if string(enqueuedRun.Payload) != `{"step":2}` {
		t.Fatalf("expected custom payload, got %s", string(enqueuedRun.Payload))
	}
}

func TestHandleSDKContinue_InheritsPayload(t *testing.T) {
	t.Parallel()
	var enqueuedRun *domain.JobRun
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Payload:   json.RawMessage(`{"inherited":true}`),
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1"}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/continue", "run-parent", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if string(enqueuedRun.Payload) != `{"inherited":true}` {
		t.Fatalf("expected inherited payload, got %s", string(enqueuedRun.Payload))
	}
}

func TestHandleSDKContinue_MaxDepthExceeded(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:           id,
				JobID:        "job-1",
				ProjectID:    "proj-1",
				Status:       domain.StatusExecuting,
				LineageDepth: 10,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/continue", "run-parent", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKContinue_InvalidStatus(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     id,
				JobID:  "job-1",
				Status: domain.StatusCompleted,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/continue", "run-parent", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKContinue_RunNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/continue", "run-parent", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKContinue_EnqueueError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1"}, nil
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
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/continue", "run-parent", `{}`)

	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
