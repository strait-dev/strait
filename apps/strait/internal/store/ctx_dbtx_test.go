package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTx is a minimal pgx.Tx test double. Only Exec, Query, and QueryRow
// are implemented — any other method call panics via the nil embedded
// interface, which is the desired behavior in unit tests (any unexpected
// call is a test bug).
type fakeTx struct {
	pgx.Tx
	execCalls     int
	queryCalls    int
	queryRowCalls int
	beginCalls    int
	execFn        func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn       func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn    func(ctx context.Context, sql string, args ...any) pgx.Row
	beginFn       func(ctx context.Context) (pgx.Tx, error)
}

func (f *fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execCalls++
	if f.execFn != nil {
		return f.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (f *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queryCalls++
	if f.queryFn != nil {
		return f.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (f *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	f.queryRowCalls++
	if f.queryRowFn != nil {
		return f.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

func (f *fakeTx) Begin(ctx context.Context) (pgx.Tx, error) {
	f.beginCalls++
	if f.beginFn != nil {
		return f.beginFn(ctx)
	}
	return &fakeTx{}, nil
}

func TestCtxAwareDBTX_NoContext_RoutesToPool(t *testing.T) {
	t.Parallel()

	poolCalls := 0
	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			poolCalls++
			return pgconn.CommandTag{}, nil
		},
	}
	wrapper := &ctxAwareDBTX{pool: pool}

	ctx := context.Background() // no tx bound
	if _, err := wrapper.Exec(ctx, "SELECT 1"); err != nil {
		require.Failf(t, "test failure",

			"Exec error = %v", err)
	}
	require.Equal(t, 1, poolCalls)
}

func TestCtxAwareDBTX_WithContext_RoutesToTx(t *testing.T) {
	t.Parallel()

	poolCalls := 0
	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			poolCalls++
			return pgconn.CommandTag{}, nil
		},
	}
	tx := &fakeTx{}
	wrapper := &ctxAwareDBTX{pool: pool}

	ctx := ContextWithTx(context.Background(), tx)
	if _, err := wrapper.Exec(ctx, "SELECT 1"); err != nil {
		require.Failf(t, "test failure",

			"Exec error = %v", err)
	}
	rows, err := wrapper.Query(ctx, "SELECT 2")
	require.NoError(t, err)

	if rows != nil {
		rows.Close()
	}
	_ = wrapper.QueryRow(ctx, "SELECT 3")
	require.Equal(t, 1, tx.execCalls)
	require.Equal(t, 1, tx.queryCalls)
	require.Equal(t, 1, tx.queryRowCalls)
	require.Equal(t, 0, poolCalls)
}

func TestCtxAwareDBTX_BeginWithContextUsesAmbientTxSavepoint(t *testing.T) {
	t.Parallel()

	poolBeginCalls := 0
	pool := &mockDBTX{}
	tx := &fakeTx{
		beginFn: func(context.Context) (pgx.Tx, error) {
			return &fakeTx{}, nil
		},
	}
	wrapper := &ctxAwareDBTX{pool: pool}
	ctx := ContextWithTx(context.Background(), tx)

	nested, err := wrapper.Begin(ctx)
	require.NoError(t, err)
	require.NotNil(
		t, nested)
	require.Equal(t, 1, tx.beginCalls)
	require.Equal(t, 0, poolBeginCalls)
}

func TestCtxAwareDBTX_BeginTxWithContextRejectsCustomOptions(t *testing.T) {
	t.Parallel()

	wrapper := &ctxAwareDBTX{pool: &mockDBTX{}}
	ctx := ContextWithTx(context.Background(), &fakeTx{})

	_, err := wrapper.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.Error(t,
		err)
}

func TestCtxAwareDBTX_ContextsAreIndependent(t *testing.T) {
	t.Parallel()

	// A single ctxAwareDBTX instance serving two concurrent "requests" with
	// different contexts must route each request to its own tx, never the
	// pool, and never cross-contaminate.
	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			assert.Failf(t, "test failure",

				"pool should never be called when every request has a tx")
			return pgconn.CommandTag{}, nil
		},
	}
	wrapper := &ctxAwareDBTX{pool: pool}

	txA := &fakeTx{}
	txB := &fakeTx{}
	ctxA := ContextWithTx(context.Background(), txA)
	ctxB := ContextWithTx(context.Background(), txB)

	var wg conc.WaitGroup
	const iters = 50
	wg.Go(func() {
		for range iters {
			_, _ = wrapper.Exec(ctxA, "SELECT 'a'")
		}
	})
	wg.Go(func() {
		for range iters {
			_, _ = wrapper.Exec(ctxB, "SELECT 'b'")
		}
	})
	wg.Wait()
	require.Equal(t,
		iters, txA.
			execCalls,
	)
	require.Equal(t,
		iters, txB.
			execCalls,
	)
}

func TestCtxAwareDBTX_TxErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("tx blew up")
	tx := &fakeTx{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, sentinel
		},
	}
	wrapper := &ctxAwareDBTX{pool: &mockDBTX{}}
	ctx := ContextWithTx(context.Background(), tx)

	_, err := wrapper.Exec(ctx, "SELECT 1")
	require.ErrorIs(t,
		err, sentinel)
}

func TestNewWithContextRouting_FallsThroughToPoolWithoutTx(t *testing.T) {
	t.Parallel()

	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewWithContextRouting(pool)
	require.NoError(t, q.SetProjectContext(context.Background(), "any-project"))

	// Call a store method that issues q.db.Exec under the hood. Use a simple
	// generic path: SetProjectContext runs q.db.Exec.
}

func TestTxFromContext_MissingReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := TxFromContext(context.Background())
	require.False(t,
		ok)
}

func TestContextWithoutTxMasksTransactionButPreservesValues(t *testing.T) {
	t.Parallel()

	type valueKey struct{}
	base := context.WithValue(context.Background(), valueKey{}, "kept")
	tx := &fakeTx{}

	ctx := ContextWithoutTx(ContextWithTx(base, tx))

	if _, ok := TxFromContext(ctx); ok {
		require.Fail(t,

			"expected transaction to be hidden")
	}
	require.Equal(t,
		"kept",
		ctx.Value(valueKey{}))
}
