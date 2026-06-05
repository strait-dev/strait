package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// fakeNoRowsRow returns pgx.ErrNoRows from Scan so the advisory-lock path
// falls into the "row expired between INSERT and SELECT" branch.
type fakeNoRowsRow struct{}

func (fakeNoRowsRow) Scan(_ ...any) error { return pgx.ErrNoRows }

type idempotencyDeleteErrBeginner struct {
	tx *idempotencyDeleteErrTx
}

func (b *idempotencyDeleteErrBeginner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("idempotencyDeleteErrBeginner: Exec should not be called outside transaction")
}

func (b *idempotencyDeleteErrBeginner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("idempotencyDeleteErrBeginner: Query should not be called outside transaction")
}

func (b *idempotencyDeleteErrBeginner) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeNoRowsRow{}
}

func (b *idempotencyDeleteErrBeginner) Begin(context.Context) (pgx.Tx, error) {
	return b.tx, nil
}

// idempotencyDeleteErrTx drives the advisory-lock path through:
// advisory lock -> insert no row -> select no row -> delete failure.
type idempotencyDeleteErrTx struct {
	pgx.Tx
	deleteErr  error
	calls      []string
	rolledBack bool
}

func (d *idempotencyDeleteErrTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "pg_advisory_xact_lock"):
		d.calls = append(d.calls, "advisory_lock")
		return pgconn.CommandTag{}, nil
	case strings.Contains(sql, "DELETE FROM idempotency_keys"):
		d.calls = append(d.calls, "delete")
		return pgconn.CommandTag{}, d.deleteErr
	default:
		return pgconn.CommandTag{}, errors.New("idempotencyDeleteErrTx: unexpected exec: " + sql)
	}
}

func (d *idempotencyDeleteErrTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("idempotencyDeleteErrTx: query not supported")
}

func (d *idempotencyDeleteErrTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "INSERT INTO idempotency_keys"):
		d.calls = append(d.calls, "insert")
	case strings.Contains(sql, "SELECT status"):
		d.calls = append(d.calls, "select")
	default:
		d.calls = append(d.calls, "unexpected_query_row")
	}
	return fakeNoRowsRow{}
}

func (d *idempotencyDeleteErrTx) Commit(context.Context) error {
	d.calls = append(d.calls, "commit")
	return nil
}

func (d *idempotencyDeleteErrTx) Rollback(context.Context) error {
	d.rolledBack = true
	return nil
}

func (d *idempotencyDeleteErrTx) attemptedRetryInsertAfterDelete() bool {
	seenDelete := false
	for _, call := range d.calls {
		if call == "delete" {
			seenDelete = true
			continue
		}
		if seenDelete && call == "insert" {
			return true
		}
	}
	return false
}

func (d *idempotencyDeleteErrTx) hasCall(name string) bool {
	for _, call := range d.calls {
		if call == name {
			return true
		}
	}
	return false
}

// TestTryAcquireIdempotencyKeyLegacy_DeletePropagatesError verifies the
// expired-row fallback returns the DELETE error to the caller instead of
// continuing to the retry INSERT on a possibly-broken connection.
// Pre-fix, the discarded error masked a torn DB connection and the
// follow-up INSERT would fail with a confusing "retry insert" error
// instead of the root cause.
func TestTryAcquireIdempotencyKeyLegacy_DeletePropagatesError(t *testing.T) {
	t.Parallel()

	rootCause := errors.New("connection refused mid-delete")
	tx := &idempotencyDeleteErrTx{deleteErr: rootCause}
	q := New(&idempotencyDeleteErrBeginner{tx: tx})

	_, _, _, _, err := q.TryAcquireIdempotencyKey(context.Background(), "proj-1", "key-1", time.Minute)
	require.Error(t,
		err)
	require.ErrorIs(t,
		err, rootCause)
	require.Contains(t,
		err.
			Error(), "delete expired idempotency key")

	// Belt-and-suspenders: the function must NOT have attempted the retry
	// insert after a failed delete — otherwise the original error semantics
	// (broken connection masquerading as a transient conflict) creep back in.
	require.True(t, tx.hasCall("delete"))
	require.False(t, tx.attemptedRetryInsertAfterDelete())
	require.True(t, tx.rolledBack)
	require.False(t, tx.hasCall("commit"))
}
