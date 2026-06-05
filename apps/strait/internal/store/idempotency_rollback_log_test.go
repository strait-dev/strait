package store

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// TestLogIdempotencyRollbackErr_NilNoLog verifies the helper is a no-op on
// the happy path (committed transactions and clean rollbacks).
func TestLogIdempotencyRollbackErr_NilNoLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	logIdempotencyRollbackErr(nil)
	require.EqualValues(t, 0, buf.
		Len())

}

// TestLogIdempotencyRollbackErr_TxClosedNoLog ensures we don't spam logs
// for the expected committed-then-deferred-rollback path. pgx returns
// ErrTxClosed from Rollback() after a successful Commit().
func TestLogIdempotencyRollbackErr_TxClosedNoLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	logIdempotencyRollbackErr(pgx.ErrTxClosed)
	require.EqualValues(t, 0, buf.
		Len())

}

// TestLogIdempotencyRollbackErr_TxClosedWrappedNoLog verifies that wrapped
// ErrTxClosed errors are also suppressed — pgx callers occasionally wrap
// the sentinel before bubbling it up.
func TestLogIdempotencyRollbackErr_TxClosedWrappedNoLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	wrapped := errors.Join(errors.New("outer"), pgx.ErrTxClosed)
	logIdempotencyRollbackErr(wrapped)
	require.EqualValues(t, 0, buf.
		Len())

}

// TestLogIdempotencyRollbackErr_RealErrorEmitsWarn proves the helper does
// what it claims for a genuine rollback failure: emits a WARN with the
// error message attached. This is the operational signal we lost when the
// previous code used `_ = tx.Rollback(ctx)`.
func TestLogIdempotencyRollbackErr_RealErrorEmitsWarn(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	logIdempotencyRollbackErr(errors.New("connection reset by peer"))

	out := buf.String()
	require.True(
		t, strings.Contains(out,
			"level=WARN",
		))
	require.True(
		t, strings.Contains(out,
			"failed to rollback idempotency transaction",
		))
	require.True(
		t, strings.Contains(out,
			"connection reset by peer",
		))

}
