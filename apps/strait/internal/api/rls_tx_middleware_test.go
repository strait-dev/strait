package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if w.Header().Get("X-Test-Header") != "" {
		t.Fatalf("success header leaked after failed commit: %q", w.Header().Get("X-Test-Header"))
	}
	if got := w.Body.String(); got == "success body" {
		t.Fatal("success body leaked after failed commit")
	}
	if !tx.setProjectContext {
		t.Fatal("RLS project context was not set")
	}
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

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	if tx.commitCalled {
		t.Fatal("transaction committed after response buffer overflow")
	}
	if !tx.rollbackCalled {
		t.Fatal("transaction was not rolled back after response buffer overflow")
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "streamed audit export" {
		t.Fatalf("body = %q", got)
	}
	if beginCount.Load() != 0 {
		t.Fatalf("RLS tx began %d times for audit export, want 0", beginCount.Load())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "webhook tested" {
		t.Fatalf("body = %q", got)
	}
	if beginCount.Load() != 0 {
		t.Fatalf("RLS tx began %d times for webhook test, want 0", beginCount.Load())
	}
}

type fakeTxBeginner struct {
	tx         *fakeRLSTx
	beginCount *atomic.Int32
}

func (f fakeTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	if f.beginCount != nil {
		f.beginCount.Add(1)
	}
	return f.tx, nil
}

type fakeRLSTx struct {
	commitErr         error
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
	return pgconn.CommandTag{}, nil
}
func (f *fakeRLSTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRLSTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (f *fakeRLSTx) Conn() *pgx.Conn                                  { return nil }
