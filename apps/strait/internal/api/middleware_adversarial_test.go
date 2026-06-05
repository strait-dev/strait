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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
)

// 1. projectContextMiddleware

// mockProjectContextSetter implements ProjectContextSetter for testing.
type mockProjectContextSetter struct {
	APIStore
	setErr   error
	clearErr error
	setCalls int
}

func (m *mockProjectContextSetter) SetProjectContext(_ context.Context, _ string) error {
	m.setCalls++
	return m.setErr
}

func (m *mockProjectContextSetter) ClearProjectContext(_ context.Context) error {
	return m.clearErr
}

func TestProjectContextMiddleware_NoProjectID(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{APIStore: &APIStoreMock{}}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No project ID in context -- middleware should still call next.
	handler.ServeHTTP(w, r)
	require.True(
		t, called)
	require.EqualValues(t, 0, setter.setCalls)

}

func TestProjectContextMiddleware_WithProjectID(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{APIStore: &APIStoreMock{}}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	handler.ServeHTTP(w, r)
	require.True(
		t, called)
	require.EqualValues(t, 1, setter.setCalls)

}

func TestProjectContextMiddleware_SetError(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{
		APIStore: &APIStoreMock{},
		setErr:   errors.New("db connection lost"),
	}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	handler.ServeHTTP(w, r)
	require.True(
		t, called)

	// Even on SetProjectContext error, the middleware should still call next.

}

func TestProjectContextMiddleware_ClearError(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{
		APIStore: &APIStoreMock{},
		clearErr: errors.New("clear failed"),
	}
	srv := &Server{store: setter}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	// Should not panic even when ClearProjectContext returns an error.
	handler.ServeHTTP(w, r)
	require.True(
		t, called)

}

func TestProjectContextMiddleware_StoreDoesNotImplementSetter(t *testing.T) {
	t.Parallel()
	// When the store does not implement ProjectContextSetter, the middleware
	// should pass through directly (return next).
	srv := &Server{store: &APIStoreMock{}}

	called := false
	handler := srv.projectContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(r.Context(), ctxProjectIDKey, "proj-123")
	r = r.WithContext(ctx)

	handler.ServeHTTP(w, r)
	require.True(
		t, called)

}

// 1b. rlsTxMiddleware

// rlsFakeTx is a minimal pgx.Tx stub for testing rlsTxMiddleware. Only the
// methods the middleware calls (Exec, Commit, Rollback) are implemented.
type rlsFakeTx struct {
	pgx.Tx
	execErr       error
	commitErr     error
	rollbackErr   error
	execCalls     int
	commitCalls   int
	rollbackCalls int
}

func (f *rlsFakeTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	f.execCalls++
	return pgconn.CommandTag{}, f.execErr
}

func (f *rlsFakeTx) Commit(_ context.Context) error {
	f.commitCalls++
	return f.commitErr
}

func (f *rlsFakeTx) Rollback(_ context.Context) error {
	f.rollbackCalls++
	return f.rollbackErr
}

// rlsFakeTxBeginner implements store.TxBeginner for rlsTxMiddleware tests.
type rlsFakeTxBeginner struct {
	tx       pgx.Tx
	beginErr error
	calls    int
}

func (b *rlsFakeTxBeginner) Begin(_ context.Context) (pgx.Tx, error) {
	b.calls++
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

func TestRLSTxMiddleware_NoProjectID_PassThrough(t *testing.T) {
	t.Parallel()
	tx := &rlsFakeTx{}
	srv := &Server{txPool: &rlsFakeTxBeginner{tx: tx}, store: &APIStoreMock{}}

	called := false
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.True(
		t, called)
	require.False(t, tx.execCalls !=
		0 || tx.
		commitCalls !=
		0)
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestRLSTxMiddleware_HappyPath_BeginsSetsConfigCommits(t *testing.T) {
	t.Parallel()
	tx := &rlsFakeTx{}
	pool := &rlsFakeTxBeginner{tx: tx}
	srv := &Server{txPool: pool, store: &APIStoreMock{}}

	called := false
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Verify the request context carries the bound tx.
		gotTx, ok := store.TxFromContext(r.Context())
		assert.True(t,
			ok)
		assert.Equal(
			t, tx, gotTx)

		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-123"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.True(
		t, called)
	require.EqualValues(t, 1, pool.calls)
	require.EqualValues(t, 1, tx.execCalls)
	require.EqualValues(t, 1, tx.commitCalls)
	require.EqualValues(t, 0, tx.rollbackCalls)
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestRLSTxMiddleware_BeginFails_FailsClosed(t *testing.T) {
	t.Parallel()
	pool := &rlsFakeTxBeginner{beginErr: errors.New("pool exhausted")}
	srv := &Server{txPool: pool, store: &APIStoreMock{}}

	called := false
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-123"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.False(t, called)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestRLSTxMiddleware_SetConfigFails_RollsBackAnd500(t *testing.T) {
	t.Parallel()
	tx := &rlsFakeTx{execErr: errors.New("exec failed")}
	srv := &Server{txPool: &rlsFakeTxBeginner{tx: tx}, store: &APIStoreMock{}}

	called := false
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-123"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.False(t, called)
	require.EqualValues(t, 1, tx.rollbackCalls)
	require.EqualValues(t, 0, tx.commitCalls)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestRLSTxMiddleware_HandlerPanic_RollsBack(t *testing.T) {
	t.Parallel()
	tx := &rlsFakeTx{}
	srv := &Server{txPool: &rlsFakeTxBeginner{tx: tx}, store: &APIStoreMock{}}

	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("handler blew up")
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-123"))
	w := httptest.NewRecorder()

	defer func() {
		require.NotNil(t, recover())
		require.EqualValues(t, 1, tx.rollbackCalls)
		require.EqualValues(t, 0, tx.commitCalls)

	}()
	handler.ServeHTTP(w, r)
}

func TestRLSTxMiddleware_NoTxPool_FallsBackToLegacy(t *testing.T) {
	t.Parallel()
	setter := &mockProjectContextSetter{APIStore: &APIStoreMock{}}
	srv := &Server{txPool: nil, store: setter}

	called := false
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-123"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.True(
		t, called)
	require.EqualValues(t, 1, setter.setCalls)

}

// 2. requestMetrics

func TestRequestMetrics_NilMetrics(t *testing.T) {
	t.Parallel()
	srv := &Server{metrics: nil}

	called := false
	handler := srv.requestMetrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(w, r)
	require.True(
		t, called)
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestRequestMetrics_RecordsStatusOnSuccess(t *testing.T) {
	t.Parallel()
	// With nil metrics, the middleware skips instrumentation but still serves.
	// This verifies the pass-through path works correctly and records the
	// right status code on the wrapped response writer.
	srv := &Server{metrics: nil}

	handler := srv.requestMetrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	handler.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

}

func TestRequestMetrics_RecordsErrorStatus(t *testing.T) {
	t.Parallel()
	srv := &Server{metrics: nil}

	handler := srv.requestMetrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

// 3. normalizeAPIError

func TestNormalizeAPIError_StringInput(t *testing.T) {
	t.Parallel()
	got := normalizeAPIError(http.StatusBadRequest, "bad input")
	require.Equal(t, ErrorCodeBadRequest,
		got.
			Code)
	require.Equal(t, "bad input",
		got.Message,
	)

}

func TestNormalizeAPIError_EmptyString(t *testing.T) {
	t.Parallel()
	got := normalizeAPIError(http.StatusNotFound, "")
	require.Equal(t, ErrorCodeNotFound,
		got.
			Code)
	require.Equal(t, "Not Found",
		got.Message,
	)

}

func TestNormalizeAPIError_NilError(t *testing.T) {
	t.Parallel()
	var err error
	got := normalizeAPIError(http.StatusInternalServerError, err)
	require.Equal(t, ErrorCodeInternalError,

		got.Code,
	)
	require.Equal(t, "Internal Server Error",

		got.Message,
	)

}

func TestNormalizeAPIError_NonNilError(t *testing.T) {
	t.Parallel()
	got := normalizeAPIError(http.StatusBadRequest, errors.New("field missing"))
	require.Equal(t, ErrorCodeBadRequest,
		got.
			Code)
	require.Equal(t, "field missing",
		got.Message,
	)

}

func TestNormalizeAPIError_APIErrorValue(t *testing.T) {
	t.Parallel()
	ae := APIError{Code: "custom_code", Message: "custom message"}
	got := normalizeAPIError(http.StatusBadRequest, ae)
	require.Equal(t, "custom_code",
		got.Code,
	)
	require.Equal(t, "custom message",
		got.Message,
	)

}

func TestNormalizeAPIError_APIErrorEmptyCode(t *testing.T) {
	t.Parallel()
	ae := APIError{Message: "some message"}
	got := normalizeAPIError(http.StatusNotFound, ae)
	require.Equal(t, ErrorCodeNotFound,
		got.
			Code)

}

func TestNormalizeAPIError_APIErrorEmptyMessage(t *testing.T) {
	t.Parallel()
	ae := APIError{Code: "custom"}
	got := normalizeAPIError(http.StatusForbidden, ae)
	require.Equal(t, "Forbidden",
		got.Message,
	)

}

func TestNormalizeAPIError_APIErrorPointer(t *testing.T) {
	t.Parallel()
	ae := &APIError{Code: "ptr_code", Message: "ptr_msg"}
	got := normalizeAPIError(http.StatusConflict, ae)
	require.False(t, got.Code != "ptr_code" ||
		got.
			Message != "ptr_msg",
	)

}

func TestNormalizeAPIError_NilAPIErrorPointer(t *testing.T) {
	t.Parallel()
	var ae *APIError
	got := normalizeAPIError(http.StatusBadRequest, ae)
	require.Equal(t, ErrorCodeBadRequest,
		got.
			Code)
	require.Equal(t, "Bad Request",
		got.Message,
	)

}

func TestNormalizeAPIError_UnknownType(t *testing.T) {
	t.Parallel()
	got := normalizeAPIError(http.StatusTeapot, 42)
	require.Equal(t, ErrorCodeInternalError,

		got.Code,
	)
	require.Equal(t, "I'm a teapot",
		got.Message,
	)

}

func TestNormalizeAPIError_WrappedError(t *testing.T) {
	t.Parallel()
	inner := errors.New("root cause")
	wrapped := fmt.Errorf("outer: %w", inner)
	got := normalizeAPIError(http.StatusInternalServerError, wrapped)
	require.Equal(t, "outer: root cause",
		got.
			Message,
	)

}

func TestNormalizeAPIError_JoinedErrors(t *testing.T) {
	t.Parallel()
	joined := errors.Join(errors.New("err1"), errors.New("err2"))
	got := normalizeAPIError(http.StatusBadRequest, joined)
	require.False(t, !strings.Contains(got.Message,

		"err1") ||
		!strings.Contains(got.
			Message, "err2",
		))

}

// 4. validateTriggerRequest (dry-run validation)

func newTriggerTestServer(t *testing.T, ms *APIStoreMock) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	var p pubsub.Publisher
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Queue:   &mockQueue{},
		PubSub:  p,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestValidateTriggerRequest_Valid(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{Payload: json.RawMessage(`{"key":"value"}`)}
	result, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "job-1", result.
		Job.ID)

}

func TestValidateTriggerRequest_EmptyJobID(t *testing.T) {
	t.Parallel()
	srv := newTriggerTestServer(t, &APIStoreMock{})

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "", req)
	require.Error(t, err)

}

func TestValidateTriggerRequest_WhitespaceJobID(t *testing.T) {
	t.Parallel()
	srv := newTriggerTestServer(t, &APIStoreMock{})

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "   ", req)
	require.Error(t, err)

}

func TestValidateTriggerRequest_JobNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-missing", req)
	require.Error(t, err)

}

func TestValidateTriggerRequest_JobDisabled(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: false}, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "disabled"))

}

func TestValidateTriggerRequest_JobPaused(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, Paused: true}, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "paused"))

}

func TestValidateTriggerRequest_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 30}, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	// maxPayloadSize is 5MB; generate something larger.
	largePayload := `{"data":"` + strings.Repeat("x", 6*1024*1024) + `"}`
	req := TriggerRequest{Payload: json.RawMessage(largePayload)}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "too large"))

}

func TestValidateTriggerRequest_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, errors.New("database unavailable")
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	require.Error(t, err)

}

func TestValidateTriggerRequest_QuotaExceeded(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 30}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxQueuedRuns: 5}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
	}
	srv := newTriggerTestServer(t, ms)

	req := TriggerRequest{}
	_, err := srv.validateTriggerRequest(context.Background(), "job-1", req)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "quota"))

}

// 5. handleCreateProject

func TestHandleCreateProject_StoreError_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			return errors.New("connection refused")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"Test Project"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestHandleCreateProject_DuplicateName_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			return errors.New("duplicate key value violates unique constraint")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-dup","org_id":"org-1","name":"Duplicate"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

	// Without special duplicate handling, the store error maps to 500.

}

func TestHandleCreateProject_ProjectLimitExceeded_Adversarial(t *testing.T) {
	t.Parallel()
	limitErr := &billing.LimitError{
		Code:         "project_limit_exceeded",
		Message:      "project limit reached",
		CurrentUsage: 5,
		Limit:        5,
		Plan:         "free",
		UpgradeURL:   "https://example.com/upgrade",
	}

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}

	ms := &APIStoreMock{
		// CreateProject should not be called when limit check fails.
		CreateProjectFunc: func(_ context.Context, _ *domain.Project) error {
			require.Fail(t,

				"CreateProject should not be called when limit is exceeded")
			return nil
		},
	}

	enforcer := &adversarialBillingEnforcer{
		checkProjectLimitFn: func(_ context.Context, _ string) error {
			return limitErr
		},
	}

	srv := NewServer(ServerDeps{
		Config:          cfg,
		Store:           ms,
		Queue:           &mockQueue{},
		Edition:         domain.EditionCloud,
		BillingEnforcer: enforcer,
	})
	t.Cleanup(srv.Close)

	body := `{"id":"proj-new","org_id":"org-1","name":"Over Limit"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", body))
	require.Equal(t, http.StatusPaymentRequired,

		w.
			Code)

	var resp QuotaExceededBody
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &resp))
	require.Equal(t, "quota_exceeded",
		resp.
			Code)
	require.Equal(t, "project_limit_exceeded",

		resp.
			Kind)
	require.Equal(t, "project limit reached",

		resp.
			Message)

}

func TestHandleCreateProject_InvalidBody_Adversarial(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", `{invalid json`))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleCreateProject_EmptyBody_Adversarial(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/projects/", `{}`))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

}

func TestHandleCreateProject_ForbiddenForAPIKeyAuth_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"id":"proj-1","org_id":"org-1","name":"My Project"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/projects/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	// Simulate a request with scopes set (API key auth) via internal secret + context.
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	ctx := context.WithValue(r.Context(), ctxScopesKey, []string{"projects:write"})
	r = r.WithContext(ctx)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden,
		w.
			Code)

}

// 6. handleDeleteSecret

func TestHandleDeleteSecret_Success_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "test-project", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, id string) error {
			require.Equal(t, "sec-123", id)

			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-123", ""))
	require.Equal(t, http.StatusNoContent,
		w.
			Code)

}

func TestHandleDeleteSecret_NotFound_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, _ string) (*domain.JobSecret, error) {
			return nil, store.ErrJobSecretNotFound
		},
		DeleteJobSecretFunc: func(_ context.Context, _ string) error {
			return store.ErrJobSecretNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-missing", ""))
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

}

func TestHandleDeleteSecret_StoreError_Adversarial(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: id, ProjectID: "test-project", SecretKey: "KEY"}, nil
		},
		DeleteJobSecretFunc: func(_ context.Context, _ string) error {
			return errors.New("unexpected IO error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/secrets/sec-err", ""))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

// Additional edge cases for normalizeAPIError / defaultErrorCode

func TestDefaultErrorCode_AllStatusCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, ErrorCodeBadRequest},
		{http.StatusUnauthorized, ErrorCodeAuthenticationRequired},
		{http.StatusForbidden, ErrorCodeForbidden},
		{http.StatusNotFound, ErrorCodeNotFound},
		{http.StatusConflict, ErrorCodeConflict},
		{http.StatusUnprocessableEntity, ErrorCodeValidationFailed},
		{http.StatusTooManyRequests, ErrorCodeRateLimited},
		{http.StatusInternalServerError, ErrorCodeInternalError},
		{http.StatusServiceUnavailable, ErrorCodeServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			t.Parallel()
			code := defaultErrorCode(tc.status)
			require.Equal(t, tc.want, code)

		})
	}
}

func TestNormalizeAPIError_APIErrorPointerEmptyCodeAndMessage(t *testing.T) {
	t.Parallel()
	ae := &APIError{}
	got := normalizeAPIError(http.StatusConflict, ae)
	require.Equal(t, ErrorCodeConflict,
		got.
			Code)
	require.Equal(t, "Conflict", got.
		Message,
	)

}

// Cross-org access via requireProjectMatch

func TestRequireProjectMatch_SameProject(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	err := requireProjectMatch(ctx, "proj-1")
	require.NoError(t, err)

}

func TestRequireProjectMatch_DifferentProject(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	err := requireProjectMatch(ctx, "proj-2")
	require.True(
		t, errors.Is(err,
			errProjectMismatch,
		))

}

func TestRequireProjectMatch_NoProjectContext(t *testing.T) {
	t.Parallel()
	// Internal callers without project context should pass through.
	err := requireProjectMatch(context.Background(), "proj-1")
	require.NoError(t, err)

}

// ScheduledAt validation via trigger handler

func TestTriggerJob_ScheduledAtInThePast(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, pastTime)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestTriggerJob_ScheduledAtFarFuture(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 30,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	futureTime := time.Now().Add(60 * 24 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, futureTime)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

// Helper: mock billing enforcer

// adversarialBillingEnforcer satisfies the BillingEnforcer interface for tests.
type adversarialBillingEnforcer struct {
	checkProjectLimitFn     func(ctx context.Context, orgID string) error
	ensureOrgSubscriptionFn func(ctx context.Context, orgID string) error
}

func (m *adversarialBillingEnforcer) CheckProjectLimit(ctx context.Context, orgID string) error {
	if m.checkProjectLimitFn != nil {
		return m.checkProjectLimitFn(ctx, orgID)
	}
	return nil
}

func (m *adversarialBillingEnforcer) CheckMemberLimit(_ context.Context, _ string) error {
	return nil
}

func (m *adversarialBillingEnforcer) CheckOrgCreationLimit(_ context.Context, _ string, _ domain.PlanTier) error {
	return nil
}

func (m *adversarialBillingEnforcer) CheckProjectBudgetLimit(_ context.Context, _ string) error {
	return nil
}

func (m *adversarialBillingEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *adversarialBillingEnforcer) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *adversarialBillingEnforcer) GetOrgPlanLimits(_ context.Context, _ string) (billing.OrgPlanLimits, error) {
	return billing.OrgPlanLimits{}, nil
}

func (m *adversarialBillingEnforcer) GetMonthlyRunCount(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *adversarialBillingEnforcer) CheckMaxDispatchPriority(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *adversarialBillingEnforcer) EnsureOrgSubscription(ctx context.Context, orgID string) error {
	if m.ensureOrgSubscriptionFn != nil {
		return m.ensureOrgSubscriptionFn(ctx, orgID)
	}
	return nil
}

func (m *adversarialBillingEnforcer) DispatchBilling(_ context.Context, _ string, _ domain.PlanTier, _ string, _ map[string]any) {
}
