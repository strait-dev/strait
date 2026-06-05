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

// fakeNoRowsRow returns pgx.ErrNoRows from Scan so the legacy path falls
// into the "row expired between INSERT and SELECT" branch.
type fakeNoRowsRow struct{}

func (fakeNoRowsRow) Scan(_ ...any) error { return pgx.ErrNoRows }

// legacyDeleteErrDBTX is a minimal DBTX that drives the legacy path
// through: insert (0 rows affected) -> select (ErrNoRows) -> delete (FAIL).
// The test asserts the delete error is propagated rather than silently
// discarded by the previous `_, _ = q.db.Exec(...)` callsite.
type legacyDeleteErrDBTX struct {
	deleteErr error
	calls     []string
}

func (d *legacyDeleteErrDBTX) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "INSERT INTO idempotency_keys"):
		d.calls = append(d.calls, "insert")
		// 0 rows affected so the legacy path proceeds to the SELECT.
		return pgconn.CommandTag{}, nil
	case strings.Contains(sql, "DELETE FROM idempotency_keys"):
		d.calls = append(d.calls, "delete")
		return pgconn.CommandTag{}, d.deleteErr
	default:
		return pgconn.CommandTag{}, errors.New("legacyDeleteErrDBTX: unexpected exec: " + sql)
	}
}

func (d *legacyDeleteErrDBTX) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("legacyDeleteErrDBTX: query not supported")
}

func (d *legacyDeleteErrDBTX) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "SELECT status") {
		d.calls = append(d.calls, "select")
		return fakeNoRowsRow{}
	}
	return fakeNoRowsRow{}
}

// TestTryAcquireIdempotencyKeyLegacy_DeletePropagatesError verifies the
// legacy fallback returns the DELETE error to the caller instead of
// continuing to the second INSERT on a possibly-broken connection.
// Pre-fix, the discarded error masked a torn DB connection and the
// follow-up INSERT would fail with a confusing "retry insert" error
// instead of the root cause.
func TestTryAcquireIdempotencyKeyLegacy_DeletePropagatesError(t *testing.T) {
	t.Parallel()

	rootCause := errors.New("connection refused mid-delete")
	db := &legacyDeleteErrDBTX{deleteErr: rootCause}
	q := New(db)

	_, _, _, _, err := q.TryAcquireIdempotencyKey(context.Background(), "proj-1", "key-1", time.Minute)
	require.Error(t,
		err)
	require.True(t,
		errors.Is(err, rootCause))
	require.True(t,
		strings.Contains(err.
			Error(), "delete expired idempotency key",
		))

	// Belt-and-suspenders: the function must NOT have attempted the retry
	// insert after a failed delete — otherwise the original error semantics
	// (broken connection masquerading as a transient conflict) creep back in.
	for _, call := range db.calls {
		require.False(t,
			call ==
				"insert" &&
				len(db.calls) > 1 &&
				db.calls[len(db.calls)-1] ==
					"insert")

	}
}
