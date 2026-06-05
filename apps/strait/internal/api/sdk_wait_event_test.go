package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func newSDKWaitEventTestServer(t *testing.T, s APIStore) *Server {
	t.Helper()
	cfg := &config.Config{
		JWTSigningKey:       testJWTSigningKey,
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
	}
	srv := NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)
	return srv
}

func makeSDKRunToken(t *testing.T, runID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &runTokenClaims{
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

func TestHandleSDKWaitForEvent_Success(t *testing.T) {
	t.Parallel()

	var createdTrigger *domain.EventTrigger
	var statusFrom, statusTo domain.RunStatus

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from, to domain.RunStatus, _ map[string]any) error {
			statusFrom = from
			statusTo = to
			return nil
		},
		CreateEventTriggerFunc: func(_ context.Context, trigger *domain.EventTrigger) error {
			createdTrigger = trigger
			return nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"aml:app-123","timeout_secs":7200}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK,
		rr.Code)
	require.False(t, statusFrom !=
		domain.StatusExecuting ||
		statusTo !=
			domain.
				StatusWaiting,
	)
	require.NotNil(t, createdTrigger)
	require.Equal(t, "aml:app-123",
		createdTrigger.
			EventKey)
	require.Equal(t, "job_run", createdTrigger.
		SourceType)
	require.Equal(t, "run-1", createdTrigger.
		JobRunID,
	)
	require.Equal(t, 7200, createdTrigger.
		TimeoutSecs,
	)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Equal(t, "waiting", resp["status"])
	require.Equal(t, "aml:app-123",
		resp["event_key"])
}

func TestHandleSDKWaitForEvent_RunNotExecuting(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1", Status: domain.StatusCompleted}, nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key"}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusConflict,
		rr.Code,
	)
}

func TestHandleSDKWaitForEvent_MissingEventKey(t *testing.T) {
	t.Parallel()

	srv := newSDKWaitEventTestServer(t, &APIStoreMock{})

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		rr.Code)
}

func TestHandleSDKWaitForEvent_DefaultTimeout(t *testing.T) {
	t.Parallel()

	var createdTrigger *domain.EventTrigger

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		CreateEventTriggerFunc: func(_ context.Context, trigger *domain.EventTrigger) error {
			createdTrigger = trigger
			return nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key"}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK,
		rr.Code)
	require.Equal(t, domain.DefaultEventTimeoutSecs,

		createdTrigger.
			TimeoutSecs,
	)
}

func TestHandleSDKWaitForEvent_RejectsTimeoutAboveMaximum(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(context.Context, string) (*domain.JobRun, error) {
			require.Fail(t,

				"timeout must be rejected before loading run")
			return nil, nil
		},
		CreateEventTriggerFunc: func(context.Context, *domain.EventTrigger) error {
			require.Fail(t,

				"timeout above maximum must not create an event trigger")
			return nil
		},
	}
	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key","timeout_secs":2592001}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest,
		rr.
			Code)
}

func TestHandleSDKWaitForEvent_RunNotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}

	srv := newSDKWaitEventTestServer(t, ms)

	body := `{"event_key":"some-key"}`
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-1/wait-for-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeSDKRunToken(t, "run-1"))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound,
		rr.Code,
	)
}
