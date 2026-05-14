package store

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestLogIdempotencyRollbackErr_NilNoLog verifies the helper is a no-op on
// the happy path (committed transactions and clean rollbacks).
func TestLogIdempotencyRollbackErr_NilNoLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	logIdempotencyRollbackErr(nil)

	if buf.Len() != 0 {
		t.Fatalf("logIdempotencyRollbackErr(nil) emitted %q, want empty", buf.String())
	}
}

// TestLogIdempotencyRollbackErr_TxClosedNoLog ensures we don't spam logs
// for the expected committed-then-deferred-rollback path. pgx returns
// ErrTxClosed from Rollback() after a successful Commit().
func TestLogIdempotencyRollbackErr_TxClosedNoLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	logIdempotencyRollbackErr(pgx.ErrTxClosed)

	if buf.Len() != 0 {
		t.Fatalf("logIdempotencyRollbackErr(ErrTxClosed) emitted %q, want empty", buf.String())
	}
}

// TestLogIdempotencyRollbackErr_TxClosedWrappedNoLog verifies that wrapped
// ErrTxClosed errors are also suppressed — pgx callers occasionally wrap
// the sentinel before bubbling it up.
func TestLogIdempotencyRollbackErr_TxClosedWrappedNoLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	wrapped := errors.Join(errors.New("outer"), pgx.ErrTxClosed)
	logIdempotencyRollbackErr(wrapped)

	if buf.Len() != 0 {
		t.Fatalf("logIdempotencyRollbackErr(wrapped ErrTxClosed) emitted %q, want empty", buf.String())
	}
}

// TestLogIdempotencyRollbackErr_RealErrorEmitsWarn proves the helper does
// what it claims for a genuine rollback failure: emits a WARN with the
// error message attached. This is the operational signal we lost when the
// previous code used `_ = tx.Rollback(ctx)`.
func TestLogIdempotencyRollbackErr_RealErrorEmitsWarn(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	logIdempotencyRollbackErr(errors.New("connection reset by peer"))

	out := buf.String()
	if !strings.Contains(out, "level=WARN") {
		t.Fatalf("log line missing WARN level: %q", out)
	}
	if !strings.Contains(out, "failed to rollback idempotency transaction") {
		t.Fatalf("log line missing rollback message: %q", out)
	}
	if !strings.Contains(out, "connection reset by peer") {
		t.Fatalf("log line missing underlying error: %q", out)
	}
}
