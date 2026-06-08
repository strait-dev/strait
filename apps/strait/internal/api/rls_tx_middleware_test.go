package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestRLSTxMiddleware_CommitFailureDoesNotLeakSuccessResponse(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("commit failed")
	tx := &fakeRLSTx{commitErr: commitErr}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test-Header", "success")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("success body"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
	require.Empty(t, w.
		Header().Get("X-Test-Header"))
	require.NotEqual(t, "success body",

		w.Body.String())
	require.True(
		t, tx.setProjectContext,
	)
}

func TestRLSTxMiddleware_TimeoutCommitFailureReturnsRetryable429(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{commitErr: context.DeadlineExceeded}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test-Header", "success")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("success body"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.Empty(t, w.Header().Get("X-Test-Header"))
	require.NotEqual(t, "success body", w.Body.String())
	require.True(t, tx.setProjectContext)
}

func TestRLSTxMiddleware_RetryableBeginFailureReturns429(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{
		beginErr: fmt.Errorf("begin transaction: %w", retryableAdmissionErr{}),
	}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
}

func TestRLSTxMiddleware_CanceledBeginFailureReturns429(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{
		beginErr: fmt.Errorf("begin transaction: %w", context.Canceled),
	}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
}

func TestRLSTxMiddleware_ClosedConnectionBeginFailureReturns429(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{
		beginErr: errors.New("begin transaction: conn closed"),
	}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
}

func TestRLSTxMiddleware_RetryableSetConfigFailureReturns429(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{
		execErr: fmt.Errorf("set project context: %w", retryableAdmissionErr{}),
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.True(t, tx.rollbackCalled)
}

func TestRLSTxMiddleware_ClosedConnectionSetConfigFailureReturns429(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{
		execErr: errors.New("set project context: use of closed network connection"),
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.True(t, tx.rollbackCalled)
}

func TestRLSTxMiddleware_PostgresTimeoutCommitFailureReturnsRetryable429(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{commitErr: &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
}

func TestRLSTxMiddleware_SafeToRetryCommitFailureReturnsRetryable429(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{
		commitErr: fmt.Errorf("failed to deallocate cached statement(s): %w", retryableAdmissionErr{}),
	}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.True(t, tx.commitCalled)
}

func TestRLSTxMiddleware_ServerErrorResponseRollsBackWithoutCommit(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{commitErr: errors.New("commit should not run")}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondError(w, nil, http.StatusInternalServerError, "failed to enqueue run")
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.False(t, tx.commitCalled)
	require.True(t, tx.rollbackCalled)
	require.Contains(t, w.Body.String(), "failed to enqueue run")
}

func TestRLSTxMiddleware_ResponseBufferLimitRollsBack(t *testing.T) {
	t.Parallel()

	tx := &fakeRLSTx{}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxRLSBufferedResponseBytes+1)))
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge,

		w.Code,
	)
	require.False(t, tx.commitCalled)
	require.True(
		t, tx.rollbackCalled,
	)
}

func TestRLSTxMiddleware_AuditExportBypassesBufferedTransaction(t *testing.T) {
	t.Parallel()

	var beginCount atomic.Int32
	tx := &fakeRLSTx{commitErr: errors.New("should not commit")}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx, beginCount: &beginCount}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("streamed audit export"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/export", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.Equal(t, "streamed audit export",

		w.Body.String())
	require.EqualValues(t, 0, beginCount.
		Load())
}

func TestRLSTxMiddleware_WebhookTestBypassesBufferedTransaction(t *testing.T) {
	t.Parallel()

	var beginCount atomic.Int32
	tx := &fakeRLSTx{commitErr: errors.New("should not commit")}
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.txPool = fakeTxBeginner{tx: tx, beginCount: &beginCount}
	handler := srv.rlsTxMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("webhook tested"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/test", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.Equal(t, "webhook tested",

		w.Body.String())
	require.EqualValues(t, 0, beginCount.
		Load())
}

type fakeTxBeginner struct {
	tx         *fakeRLSTx
	beginCount *atomic.Int32
	beginErr   error
}

func (f fakeTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	if f.beginCount != nil {
		f.beginCount.Add(1)
	}
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return f.tx, nil
}

type fakeRLSTx struct {
	commitErr         error
	execErr           error
	setProjectContext bool
	commitCalled      bool
	rollbackCalled    bool
}

func (f *fakeRLSTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("not implemented") }
func (f *fakeRLSTx) Commit(context.Context) error {
	f.commitCalled = true
	return f.commitErr
}
func (f *fakeRLSTx) Rollback(context.Context) error {
	f.rollbackCalled = true
	return nil
}
func (f *fakeRLSTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f *fakeRLSTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (f *fakeRLSTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (f *fakeRLSTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRLSTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if sql == "SELECT set_config('app.current_project_id', $1, true)" {
		f.setProjectContext = true
	}
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return pgconn.CommandTag{}, nil
}
func (f *fakeRLSTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRLSTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (f *fakeRLSTx) Conn() *pgx.Conn                                  { return nil }

type retryableAdmissionErr struct{}

func (retryableAdmissionErr) Error() string { return "conn closed" }
func (retryableAdmissionErr) SafeToRetry() bool {
	return true
}
