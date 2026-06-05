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
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

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
	require.NoError(t, err)

	return signed
}

func TestHandleSDKLog_Success(t *testing.T) {
	t.Parallel()
	var insertCalled atomic.Bool
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			insertCalled.Store(true)
			require.Equal(t, "run-123", event.
				RunID)
			require.Equal(t, domain.EventError,
				event.
					Type)
			require.Equal(t, "warn", event.
				Level)
			require.Equal(t, "something happened",
				event.
					Message,
			)
			require.Equal(t, `{"code":123}`,
				string(
					event.Data))

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"type":"error","level":"warn","message":"something happened","data":{"code":123}}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, insertCalled.Load())
}

func TestHandleSDKLog_MissingMessage(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"type":"log"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
}

func TestHandleSDKLog_DefaultsEventType(t *testing.T) {
	t.Parallel()
	var insertCalled atomic.Bool
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			insertCalled.Store(true)
			require.Equal(t, domain.EventLog,
				event.
					Type)
			require.Equal(t, "info", event.
				Level)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/log", "run-123", `{"message":"hello"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, insertCalled.Load())
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
}

func TestHandleSDKProgress_Success(t *testing.T) {
	t.Parallel()
	var insertCalled atomic.Bool
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			insertCalled.Store(true)
			require.Equal(t, domain.EventProgress,
				event.
					Type)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/progress", "run-123", `{"percent":45,"message":"working","step":"phase-1","eta_seconds":30}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, insertCalled.Load())
}

func TestHandleSDKProgress_InvalidPercent(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/progress", "run-123", `{"percent":101,"message":"working"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
}

func TestHandleSDKAnnotate_Success(t *testing.T) {
	t.Parallel()
	updated := false
	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, id string, annotations map[string]string) error {
			updated = true
			require.Equal(t, "run-123", id)
			require.False(t, annotations["env"] != "prod" ||
				annotations["region"] !=
					"eu")

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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updated)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
}

func TestHandleSDKAnnotate_InvalidPayload(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", `{"annotations":{}}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
}

func TestHandleSDKAnnotate_TooManyAnnotations(t *testing.T) {
	t.Parallel()
	annotations := make(map[string]string)
	for i := range 51 {
		annotations[strings.Repeat("k", i+1)] = "v"
	}

	payload, err := json.Marshal(map[string]any{"annotations": annotations})
	require.NoError(t, err)

	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			require.Fail(t,

				"UpdateRunMetadata should not be called for invalid annotations")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", string(payload))

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "too many annotations (max 50)")
}

func TestHandleSDKAnnotate_AnnotationKeyTooLong(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(map[string]any{
		"annotations": map[string]string{
			strings.Repeat("k", 129): "prod",
		},
	})
	require.NoError(t, err)

	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			require.Fail(t,

				"UpdateRunMetadata should not be called for invalid annotations")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", string(payload))

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "annotation key too long (max 128 characters)")
}

func TestHandleSDKAnnotate_AnnotationValueTooLong(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(map[string]any{
		"annotations": map[string]string{
			"env": strings.Repeat("v", 1025),
		},
	})
	require.NoError(t, err)

	ms := &APIStoreMock{
		UpdateRunMetadataFunc: func(_ context.Context, _ string, _ map[string]string) error {
			require.Fail(t,

				"UpdateRunMetadata should not be called for invalid annotations")
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})
	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/annotate", "run-123", string(payload))

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.Contains(
		t, w.Body.
			String(), "annotation value too long (max 1024 characters)")
}

func TestHandleSDKCheckpoint_Success(t *testing.T) {
	t.Parallel()
	ms := &checkpointActiveRunMock{
		APIStoreMock: &APIStoreMock{
			GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
			},
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/checkpoint", "run-123", `{"state":{"cursor":12}}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, ms.created)
	require.Equal(t, "run-123", ms.created.RunID)
	require.NotEmpty(t, ms.created.State)
}

type checkpointActiveRunMock struct {
	*APIStoreMock
	created *domain.RunCheckpoint
}

func (m *checkpointActiveRunMock) GetRunTokenState(context.Context, string) (domain.RunStatus, int, string, error) {
	return domain.StatusExecuting, 1, "proj-1", nil
}

func (m *checkpointActiveRunMock) EnsureRunActiveForAttempt(context.Context, string, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) InsertEventForActiveRun(context.Context, *domain.RunEvent, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) UpdateRunMetadataForActiveRun(context.Context, string, map[string]string, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) UpdateHeartbeatForActiveRun(context.Context, string, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) CreateRunCheckpointForActiveRun(_ context.Context, checkpoint *domain.RunCheckpoint, _ int) error {
	m.created = checkpoint
	return nil
}

func (m *checkpointActiveRunMock) UpsertRunStateForActiveRun(context.Context, *domain.RunState, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) GetRunStateForActiveRun(context.Context, string, string, int) (*domain.RunState, error) {
	return nil, store.ErrRunConflict
}

func (m *checkpointActiveRunMock) ListRunStateForActiveRun(context.Context, string, int) ([]domain.RunState, error) {
	return nil, store.ErrRunConflict
}

func (m *checkpointActiveRunMock) DeleteRunStateForActiveRun(context.Context, string, string, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) UpsertRunOutputForActiveRun(context.Context, *domain.RunOutput, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) UpsertJobMemoryWithQuotaForActiveRun(context.Context, string, *domain.JobMemory, int, int, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) GetJobMemoryForActiveRun(context.Context, string, string, string, int) (*domain.JobMemory, error) {
	return nil, store.ErrRunConflict
}

func (m *checkpointActiveRunMock) ListJobMemoryForActiveRun(context.Context, string, string, int) ([]domain.JobMemory, error) {
	return nil, store.ErrRunConflict
}

func (m *checkpointActiveRunMock) DeleteJobMemoryForActiveRun(context.Context, string, string, string, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) CreateRunResourceSnapshotForActiveRun(context.Context, *domain.RunResourceSnapshot, int) error {
	return store.ErrRunConflict
}

func (m *checkpointActiveRunMock) UpdateRunStatusForActiveRun(context.Context, string, domain.RunStatus, domain.RunStatus, map[string]any, int) error {
	return store.ErrRunConflict
}

func TestSDKUsageRoute_NotRegistered(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/usage", "run-123", `{"usage_units":1}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
}

func TestHandleSDKOutput_SchemaValidation(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/output", "run-123", `{"output_key":"final","schema":{"type":"object","required":["name"]},"value":{"age":12}}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleSDKHeartbeat_Success(t *testing.T) {
	t.Parallel()
	var updateCalled atomic.Bool
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(_ context.Context, id string) error {
			updateCalled.Store(true)
			require.Equal(t, "run-123", id)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", "run-123", "")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updateCalled.Load())
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
}

func TestSDKRunToken_RevalidatesAfterDecodeBeforeMutation(t *testing.T) {
	t.Parallel()
	var statusCalls atomic.Int32
	ms := &APIStoreMock{
		UpdateHeartbeatFunc: func(context.Context, string) error {
			require.Fail(t,

				"heartbeat mutation must not run after post-decode terminal revalidation")
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
	require.Equal(t, http.StatusGone,
		w.Code,
	)
	require.EqualValues(t, 2, statusCalls.
		Load())
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
			require.Equal(t, "run-123", id)
			require.False(t, from != domain.
				StatusExecuting ||
				to !=
					domain.StatusCompleted,
			)

			if _, ok := fields["finished_at"]; !ok {
				require.Fail(t,

					"expected finished_at field")
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updateCalled.Load())
	require.Equal(t, 2, getRunCalls)
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
			require.True(
				t, ok)
			require.Equal(t, `{"ok":true}`,
				string(result))

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/complete", "run-123", `{"result":{"ok":true}}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusConflict,
		w.
			Code)
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
			require.False(t, from != domain.
				StatusExecuting ||
				to !=
					domain.StatusFailed,
			)
			require.Equal(t, "boom", fields["error"])

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{"error":"boom"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updateCalled.Load())
	require.Equal(t, 2, getRunCalls)
}

func TestHandleSDKFail_MissingError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-123/fail", "run-123", `{}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusConflict,
		w.
			Code)
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
			require.False(t, projectID !=
				"proj-1" ||
				slug != "child-job",
			)

			return &domain.Job{ID: "job-123", ProjectID: projectID, Slug: slug}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueueCalled.Store(true)
			require.Equal(t, "job-123", run.
				JobID)
			require.Equal(t, domain.TriggerSpawn,
				run.
					TriggeredBy,
			)
			require.Equal(t, "run-parent",
				run.ParentRunID,
			)
			require.Equal(t, `{"x":1}`, string(run.Payload))

			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"job_slug":"child-job","project_id":"proj-1","payload":{"x":1}}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, getJobCalled.Load())
	require.True(
		t, enqueueCalled.
			Load())
}

func TestHandleSDKSpawn_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"project_id":"proj-1"}`)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code,
	)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
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
				require.False(t, from != domain.
					StatusWaiting ||
					to !=
						domain.StatusQueued,
				)

				updatedParent = true
				return nil
			}
			if id == "run-child" && to == domain.StatusCompleted {
				return nil
			}
			return nil
		},
		AreAllDescendantsTerminalFunc: func(_ context.Context, parentRunID string) (bool, error) {
			require.Equal(t, "run-parent",
				parentRunID,
			)

			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, &mockPublisher{})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-child/complete", "run-child", `{}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, updatedParent)
}

func TestSDKAuth_MissingToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
}

func TestSDKAuth_InvalidToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer not-a-jwt")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
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
	require.Equal(t, http.StatusForbidden,
		w.
			Code)
}

func TestSDKAuth_ExpiredToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-123/heartbeat", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+generateExpiredRunToken(t, "run-123"))

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,

		w.Code)
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "2.1.0", w.Header().Get("X-SDK-Version-Accepted"))
	require.Equal(t, "progress,checkpoint",

		w.Header().Get("X-SDK-Capabilities"))
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "legacy", w.Header().Get("X-SDK-Version-Accepted"))
	require.Equal(t, "none", w.Header().Get(
		"X-SDK-Capabilities",
	))
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
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code,
	)
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
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
}

func TestHandleListRuns_Success(t *testing.T) {
	t.Parallel()
	var listCalled atomic.Bool
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, _ *domain.ExecutionMode, _ *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			listCalled.Store(true)
			require.Equal(t, "proj-1", projectID)
			require.False(t, metadataKey !=
				nil || metadataValue !=
				nil)
			require.False(t, status == nil ||
				*status !=
					domain.
						StatusExecuting)
			require.Equal(t, 101, limit)
			require.NotNil(t, cursor)

			// handler passes limit+1 for has_more detection

			return []domain.JobRun{{ID: "run-1", ProjectID: projectID, Status: domain.StatusExecuting}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	cursor := time.Now().UTC().Format(time.RFC3339)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?status=executing&limit=500&cursor="+cursor, "", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, listCalled.Load())
}

func TestHandleListRuns_MetadataFilter(t *testing.T) {
	t.Parallel()
	var listCalled atomic.Bool
	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, _ *domain.ExecutionMode, _ *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			listCalled.Store(true)
			require.Equal(t, "proj-1", projectID)
			require.Nil(t, status)
			require.False(t, metadataKey ==
				nil || *metadataKey !=
				"env")
			require.False(t, metadataValue ==
				nil ||
				*metadataValue !=
					"prod")
			require.Equal(t, 51, limit)
			require.Nil(t, cursor)

			// handler passes limit+1 (default 50 + 1)

			return []domain.JobRun{{ID: "run-1", ProjectID: projectID}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?metadata_key=env&metadata_value=prod", "", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, listCalled.Load())
}

func TestHandleListRuns_MetadataValueWithoutKey(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?metadata_value=prod", "", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListRuns_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/", "")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListRuns_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?limit=abc", "", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
}

func TestHandleListRuns_InvalidCursor(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/runs/?cursor=not-a-time", "", "proj-1")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.False(t, called)
}

func TestHandleListChildRuns_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListChildRunsFunc: func(_ context.Context, parentRunID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			require.Equal(t, "run-parent",
				parentRunID,
			)

			return []domain.JobRun{{ID: "run-child", ParentRunID: parentRunID}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodGet, "/v1/runs/run-parent/children", "")

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueuedRun)
	require.Equal(t, "run-parent",
		enqueuedRun.
			ContinuationOf,
	)
	require.Equal(t, 3, enqueuedRun.
		LineageDepth,
	)
	require.Equal(t, 5, enqueuedRun.
		Priority,
	)
	require.Equal(t, `{"step":2}`,
		string(enqueuedRun.
			Payload,
		))
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, `{"inherited":true}`,
		string(enqueuedRun.
			Payload))
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)
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
	require.Equal(t, http.StatusConflict,
		w.
			Code)
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
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
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
	require.Equal(t, http.StatusInternalServerError,

		w.Code,
	)
}
